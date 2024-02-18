package dependencies

import (
	"bufio"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/grafana/regexp"

	"github.com/sourcegraph/sourcegraph/dev/sg/internal/check"
	"github.com/sourcegraph/sourcegraph/dev/sg/internal/sgconf"
	"github.com/sourcegraph/sourcegraph/dev/sg/internal/std"
	"github.com/sourcegraph/sourcegraph/dev/sg/internal/usershell"
	"github.com/sourcegraph/sourcegraph/dev/sg/root"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

// cmdFix executes the given command as an action in a new user shell.
func cmdFix(cmd string) check.FixAction[CheckArgs] {
	return func(ctx context.Context, cio check.IO, args CheckArgs) error {
		c := usershell.Command(ctx, cmd)
		if cio.Input != nil {
			c = c.Input(cio.Input)
		}
		return c.Run().StreamLines(cio.Verbose)
	}
}

func cmdFixes(cmds ...string) check.FixAction[CheckArgs] {
	return func(ctx context.Context, cio check.IO, args CheckArgs) error {
		for _, cmd := range cmds {
			if err := cmdFix(cmd)(ctx, cio, args); err != nil {
				return err
			}
		}
		return nil
	}
}

func enableOnlyInSourcegraphRepo() check.EnableFunc[CheckArgs] {
	return func(ctx context.Context, args CheckArgs) error {
		_, err := root.RepositoryRoot()
		return err
	}
}

func disableInCI() check.EnableFunc[CheckArgs] {
	return func(ctx context.Context, args CheckArgs) error {
		// Docker is quite funky in CI
		if os.Getenv("CI") == "true" {
			return errors.New("disabled in CI")
		}
		return nil
	}
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func checkSourcegraphDatabase(ctx context.Context, out *std.Output, args CheckArgs) error {
	getConfig := func() (*sgconf.Config, error) {
		var config *sgconf.Config
		var err error
		if args.DisableOverwrite {
			config, err = sgconf.GetWithoutOverwrites(args.ConfigFile)
		} else {
			config, err = sgconf.Get(args.ConfigFile, args.ConfigOverwriteFile)
		}
		if err != nil {
			return nil, err
		}
		if config == nil {
			return nil, errors.New("failed to read sg.config.yaml. This step of `sg setup` needs to be run in the `sourcegraph` repository")
		}

		return config, nil
	}

	return check.SourcegraphDatabase(getConfig)(ctx)
}

func checkSrcCliVersion(versionConstraint string) check.CheckFunc {
	return check.CompareSemanticVersion("src", "src version -client-only", versionConstraint)
}

func forceASDFPluginAdd(ctx context.Context, plugin string, source string) error {
	err := usershell.Run(ctx, "asdf plugin-add", plugin, source).Wait()
	if err != nil && strings.Contains(err.Error(), "already added") {
		return nil
	}
	return errors.Wrap(err, "asdf plugin-add")
}

// pgUtilsPathRe is the regexp used to check what value user.bazelrc defines for
// the PG_UTILS_PATH env var.
var pgUtilsPathRe = regexp.MustCompile(`build --action_env=PG_UTILS_PATH=(.*)$`)

// userBazelRcPath is the path to a git ignored file that contains Bazel flags
// specific to the current machine that are required in certain cases.
var userBazelRcPath = ".aspect/bazelrc/user.bazelrc"

// checkPGUtilsPath ensures that a PG_UTILS_PATH is being defined in .aspect/bazelrc/user.bazelrc
// if it's needed. For example, on Linux hosts, it's usually located in /usr/bin, which is
// perfectly fine. But on Mac machines, it's either in the homebrew PATH or on a different
// location if the user installed Posgres through the Postgresql desktop app.
func checkPGUtilsPath(ctx context.Context, out *std.Output, args CheckArgs) error {
	// Check for standard PATH location, that is available inside Bazel when
	// inheriting the shell environment. That is just /usr/bin, not /usr/local/bin.
	_, err := os.Stat("/usr/bin/createdb")
	if err == nil {
		// If we have createdb in /usr/bin/, nothing to do, it will work outside the box.
		return nil
	}

	// Check for the presence of git ignored user.bazelrc, that is specific to local
	// environment. Because createdb is not under /usr/bin, we have to create that file
	// and define the PG_UTILS_PATH for migration rules.
	_, err = os.Stat(userBazelRcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Wrapf(err, "%s doesn't exist", userBazelRcPath)
		}
		return errors.Wrapf(err, "unexpected error with %s", userBazelRcPath)
	}

	// If it exists, we check if the injected PATH actually contains createdb as intended.
	// If not, we'll raise an error for sg setup to correct.
	f, err := os.Open(userBazelRcPath)
	if err != nil {
		return errors.Wrapf(err, "can't open %s", userBazelRcPath)
	}
	defer f.Close()

	err, pgUtilsPath := parsePgUtilsPathInUserBazelrc(f)
	if err != nil {
		return errors.Wrapf(err, "can't parse %s", userBazelRcPath)
	}

	// If the file exists, but doesn't reference PG_UTILS_PATH, that's an error as well.
	if pgUtilsPath == "" {
		return errors.Newf("none on the content in %s matched %q", userBazelRcPath, pgUtilsPathRe.String())
	}

	// Check that this path contains createdb as expected.
	if err := checkPgUtilsPathIncludesBinaries(pgUtilsPath); err != nil {
		return err
	}

	return nil
}

// parsePgUtilsPathInUserBazelrc extracts the defined path to the createdb postgresql
// utilities that are used in a the Bazel migration rules.
func parsePgUtilsPathInUserBazelrc(r io.Reader) (error, string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		matches := pgUtilsPathRe.FindStringSubmatch(line)
		if len(matches) > 1 {
			return nil, matches[1]
		}
	}
	return scanner.Err(), ""
}

// checkPgUtilsPathIncludesBinaries ensures that the given path contains createdb as expected.
func checkPgUtilsPathIncludesBinaries(pgUtilsPath string) error {
	_, err := os.Stat(path.Join(pgUtilsPath, "createdb"))
	if err != nil {
		if os.IsNotExist(err) {
			return errors.Wrap(err, "currently defined PG_UTILS_PATH doesn't include createdb")
		}
		return errors.Wrap(err, "currently defined PG_UTILS_PATH is incorrect")
	}
	return nil
}

// guessPgUtilsPath infers from the environment where the createdb binary
// is located and returns its parent folder, so it can be used to extend
// PATH for the migrations Bazel rules.
func guessPgUtilsPath(ctx context.Context) (error, string) {
	str, err := usershell.Run(ctx, "which", "createdb").String()
	if err != nil {
		return err, ""
	}
	return nil, filepath.Dir(str)
}

// brewInstall returns a FixAction that installs a brew formula.
// If the brew output contains an autofix for adding the formula to the path
// (in the case of keg-only formula), it will be automatically applied.
func createBrewInstallFix(formula string, cask bool) check.FixAction[CheckArgs] {
	return func(ctx context.Context, cio check.IO, args CheckArgs) error {
		cmd := "brew install "
		if cask {
			cmd += "--cask "
		}
		cmd += formula
		c := usershell.Command(ctx, cmd)
		if cio.Input != nil {
			c = c.Input(cio.Input)
		}

		pathAddCommandIsNext := false
		return c.Run().StreamLines(func(line string) {
			if pathAddCommandIsNext {
				matches := exportPathRegexp.FindStringSubmatch(line)
				if len(matches) != 2 {
					cio.Output.WriteWarningf("unexpected output from brew install: %q\n"+
						"was not able to automatically update $PATH. Please add this to "+
						"your path manually.", line)
				} else {
					_ = usershell.Run(
						ctx,
						"echo -e 'export PATH="+matches[1],
						">>",
						usershell.ShellConfigPath(ctx),
					).Wait()
				}
				pathAddCommandIsNext = false
			}
			if strings.Contains(line, "If you need to have "+formula+" first in your PATH, run:") {
				pathAddCommandIsNext = true
			}
			cio.Verbose(line)
		})
	}
}

var exportPathRegexp = regexp.MustCompile(`export PATH=(.*) >>`)

func caskInstall(formula string) check.FixAction[CheckArgs] {
	return createBrewInstallFix(formula, true)

}

func brewInstall(formula string) check.FixAction[CheckArgs] {
	return createBrewInstallFix(formula, false)
}
