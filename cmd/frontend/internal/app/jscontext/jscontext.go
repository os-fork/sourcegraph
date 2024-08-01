// Package jscontext contains functionality for information we pass down into
// the JS webapp.
package jscontext

import (
	"context"
	"net/http"
	"runtime"
	"slices"
	"strings"

	"github.com/graph-gophers/graphql-go"
	logger "github.com/sourcegraph/log"

	"github.com/sourcegraph/sourcegraph/cmd/frontend/enterprise"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/envvar"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/globals"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/graphqlbackend"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/assetsutil"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/app/ui/sveltekit"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/auth/providers"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/auth/userpasswd"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/internal/cody"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/webhooks"
	sgactor "github.com/sourcegraph/sourcegraph/internal/actor"
	"github.com/sourcegraph/sourcegraph/internal/authz"
	authzproviders "github.com/sourcegraph/sourcegraph/internal/authz/providers"
	"github.com/sourcegraph/sourcegraph/internal/batches"
	"github.com/sourcegraph/sourcegraph/internal/codemonitors"
	"github.com/sourcegraph/sourcegraph/internal/conf"
	"github.com/sourcegraph/sourcegraph/internal/conf/deploy"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/dotcom"
	"github.com/sourcegraph/sourcegraph/internal/env"
	"github.com/sourcegraph/sourcegraph/internal/insights"
	"github.com/sourcegraph/sourcegraph/internal/lazyregexp"
	"github.com/sourcegraph/sourcegraph/internal/licensing"
	"github.com/sourcegraph/sourcegraph/internal/notebooks"
	"github.com/sourcegraph/sourcegraph/internal/own"
	"github.com/sourcegraph/sourcegraph/internal/search/exhaustive"
	"github.com/sourcegraph/sourcegraph/internal/search/searchcontexts"
	"github.com/sourcegraph/sourcegraph/internal/siteid"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/internal/version"
	"github.com/sourcegraph/sourcegraph/schema"
)

type authProviderInfo struct {
	IsBuiltin         bool    `json:"isBuiltin"`
	NoSignIn          bool    `json:"noSignIn"`
	DisplayName       string  `json:"displayName"`
	DisplayPrefix     *string `json:"displayPrefix"`
	ServiceType       string  `json:"serviceType"`
	AuthenticationURL string  `json:"authenticationURL"`
	ServiceID         string  `json:"serviceID"`
	ClientID          string  `json:"clientID"`
	RequiredForAuthz  bool    `json:"requiredForAuthz"`
}

// GenericPasswordPolicy a generic password policy that holds password requirements
type authPasswordPolicy struct {
	Enabled                   bool `json:"enabled"`
	NumberOfSpecialCharacters int  `json:"numberOfSpecialCharacters"`
	RequireAtLeastOneNumber   bool `json:"requireAtLeastOneNumber"`
	RequireUpperAndLowerCase  bool `json:"requireUpperAndLowerCase"`
}
type UserLatestSettings struct {
	ID       int32                      `json:"id"`       // the unique ID of this settings value
	Contents graphqlbackend.JSONCString `json:"contents"` // the raw JSON (with comments and trailing commas allowed)
}
type UserOrganization struct {
	Typename    string     `json:"__typename"`
	ID          graphql.ID `json:"id"`
	Name        string     `json:"name"`
	URL         string     `json:"url"`
	SettingsURL *string    `json:"settingsURL"`
}
type UserOrganizationsConnection struct {
	Typename string             `json:"__typename"`
	Nodes    []UserOrganization `json:"nodes"`
}
type UserEmail struct {
	Email     string `json:"email"`
	IsPrimary bool   `json:"isPrimary"`
	Verified  bool   `json:"verified"`
}
type UserSession struct {
	CanSignOut bool `json:"canSignOut"`
}

type TemporarySettings struct {
	GraphQLTypename string `json:"__typename"`
	Contents        string `json:"contents"`
}

type Permission struct {
	GraphQLTypename string     `json:"__typename"`
	ID              graphql.ID `json:"id"`
	DisplayName     string     `json:"displayName"`
}

type PermissionsConnection struct {
	GraphQLTypename string       `json:"__typename"`
	Nodes           []Permission `json:"nodes"`
}
type CurrentUser struct {
	GraphQLTypename     string     `json:"__typename"`
	ID                  graphql.ID `json:"id"`
	DatabaseID          int32      `json:"databaseID"`
	Username            string     `json:"username"`
	AvatarURL           *string    `json:"avatarURL"`
	DisplayName         string     `json:"displayName"`
	SiteAdmin           bool       `json:"siteAdmin"`
	URL                 string     `json:"url"`
	SettingsURL         string     `json:"settingsURL"`
	ViewerCanAdminister bool       `json:"viewerCanAdminister"`
	TosAccepted         bool       `json:"tosAccepted"`
	HasVerifiedEmail    bool       `json:"hasVerifiedEmail"`

	Organizations  *UserOrganizationsConnection `json:"organizations"`
	Session        *UserSession                 `json:"session"`
	Emails         []UserEmail                  `json:"emails"`
	LatestSettings *UserLatestSettings          `json:"latestSettings"`
	Permissions    PermissionsConnection        `json:"permissions"`
}

// FeatureBatchChanges describes if and how the Batch Changes feature is available on
// the given license plan. It mirrors the type licensing.FeatureBatchChanges.
type FeatureBatchChanges struct {
	// If true, there is no limit to the number of changesets that can be created.
	Unrestricted bool `json:"unrestricted"`
	// Maximum number of changesets that can be created per batch change.
	// If Unrestricted is true, this is ignored.
	MaxNumChangesets int `json:"maxNumChangesets"`
}

// LicenseInfo contains non-sensitive information about the current license on the instance.
type LicenseInfo struct {
	BatchChanges *FeatureBatchChanges `json:"batchChanges"`
}

// FrontendCodyProConfig is the configuration data for Cody Pro that needs to be passed
// to the frontend.
type FrontendCodyProConfig struct {
	StripePublishableKey string `json:"stripePublishableKey"`
	SscBaseUrl           string `json:"sscBaseUrl"`
	UseEmbeddedUI        bool   `json:"useEmbeddedUI"`
}

// JSContext is made available to JavaScript code via the
// "sourcegraph/app/context" module.
//
// 🚨 SECURITY: This struct is sent to all users regardless of whether or
// not they are logged in, for example on an auth.public=false private
// server. Including secret fields here is OK if it is based on the user's
// authentication above, but do not include e.g. hard-coded secrets about
// the server instance here as they would be sent to anonymous users.
type JSContext struct {
	AppRoot        string            `json:"appRoot,omitempty"`
	ExternalURL    string            `json:"externalURL,omitempty"`
	XHRHeaders     map[string]string `json:"xhrHeaders"`
	UserAgentIsBot bool              `json:"userAgentIsBot"`
	AssetsRoot     string            `json:"assetsRoot"`
	Version        string            `json:"version"`

	IsAuthenticatedUser bool               `json:"isAuthenticatedUser"`
	CurrentUser         *CurrentUser       `json:"currentUser"`
	TemporarySettings   *TemporarySettings `json:"temporarySettings"`

	SentryDSN     *string               `json:"sentryDSN"`
	OpenTelemetry *schema.OpenTelemetry `json:"openTelemetry"`

	SiteID        string `json:"siteID"`
	SiteGQLID     string `json:"siteGQLID"`
	Debug         bool   `json:"debug"`
	NeedsSiteInit bool   `json:"needsSiteInit"`
	EmailEnabled  bool   `json:"emailEnabled"`

	Site              schema.SiteConfiguration `json:"site"` // public subset of site configuration
	NeedServerRestart bool                     `json:"needServerRestart"`
	DeployType        string                   `json:"deployType"`

	SourcegraphDotComMode bool `json:"sourcegraphDotComMode"`

	AccessTokensAllow                 conf.AccessTokenAllow `json:"accessTokensAllow"`
	AccessTokensAllowNoExpiration     bool                  `json:"accessTokensAllowNoExpiration"`
	AccessTokensDefaultExpirationDays int                   `json:"accessTokensExpirationDaysDefault"`
	AccessTokensExpirationDaysOptions []int                 `json:"accessTokensExpirationDaysOptions"`

	AllowSignup bool `json:"allowSignup"`

	ResetPasswordEnabled bool `json:"resetPasswordEnabled"`

	AuthMinPasswordLength int                `json:"authMinPasswordLength"`
	AuthPasswordPolicy    authPasswordPolicy `json:"authPasswordPolicy"`

	AuthProviders                  []authProviderInfo `json:"authProviders"`
	AuthPrimaryLoginProvidersCount int                `json:"primaryLoginProvidersCount"`

	AuthAccessRequest *schema.AuthAccessRequest `json:"authAccessRequest"`

	Branding *schema.Branding `json:"branding"`

	// BatchChangesEnabled is true if:
	// * Batch Changes is NOT disabled by a flag in the site config
	// * Batch Changes is NOT limited to admins-only, or it is, but the user issuing
	//   the request is an admin and thus can access batch changes
	// It does NOT reflect whether or not the site license has batch changes available.
	// Use LicenseInfo for that.
	BatchChangesEnabled                bool `json:"batchChangesEnabled"`
	BatchChangesDisableWebhooksWarning bool `json:"batchChangesDisableWebhooksWarning"`
	BatchChangesWebhookLogsEnabled     bool `json:"batchChangesWebhookLogsEnabled"`

	// CodyEnabledOnInstance is true `cody.enabled` is not false in site config. Check
	// CodyEnabledForCurrentUser to see if the current user has access to Cody.
	CodyEnabledOnInstance bool `json:"codyEnabledOnInstance"`

	// CodyEnabledForCurrentUser is true if CodyEnabled is true and the current
	// user has access to Cody.
	CodyEnabledForCurrentUser bool `json:"codyEnabledForCurrentUser"`

	// CodyRequiresVerifiedEmail is true if usage of Cody requires the current
	// user to have a verified email.
	CodyRequiresVerifiedEmail bool `json:"codyRequiresVerifiedEmail"`

	// CodeSearchEnabledOnInstance is true if code search is licensed. (There is currently no
	// separate config to disable it if licensed.)
	CodeSearchEnabledOnInstance bool `json:"codeSearchEnabledOnInstance"`

	ExecutorsEnabled                               bool `json:"executorsEnabled"`
	CodeIntelAutoIndexingEnabled                   bool `json:"codeIntelAutoIndexingEnabled"`
	CodeIntelAutoIndexingAllowGlobalPolicies       bool `json:"codeIntelAutoIndexingAllowGlobalPolicies"`
	CodeIntelRankingDocumentReferenceCountsEnabled bool `json:"codeIntelRankingDocumentReferenceCountsEnabled"`

	CodeInsightsEnabled      bool   `json:"codeInsightsEnabled"`
	ApplianceUpdateTarget    string `json:"applianceUpdateTarget"`
	ApplianceMenuTarget      string `json:"applianceMenuTarget"`
	CodeIntelligenceEnabled  bool   `json:"codeIntelligenceEnabled"`
	SearchContextsEnabled    bool   `json:"searchContextsEnabled"`
	NotebooksEnabled         bool   `json:"notebooksEnabled"`
	CodeMonitoringEnabled    bool   `json:"codeMonitoringEnabled"`
	SearchAggregationEnabled bool   `json:"searchAggregationEnabled"`
	OwnEnabled               bool   `json:"ownEnabled"`
	SearchJobsEnabled        bool   `json:"searchJobsEnabled"`

	RedirectUnsupportedBrowser bool `json:"RedirectUnsupportedBrowser"`

	ProductResearchPageEnabled bool `json:"productResearchPageEnabled"`

	ExperimentalFeatures schema.ExperimentalFeatures `json:"experimentalFeatures"`

	LicenseInfo LicenseInfo `json:"licenseInfo"`

	HashedLicenseKey string `json:"hashedLicenseKey"`

	OutboundRequestLogLimit int `json:"outboundRequestLogLimit"`

	DisableFeedbackSurvey bool `json:"disableFeedbackSurvey"`

	NeedsRepositoryConfiguration bool `json:"needsRepositoryConfiguration"`

	ExtsvcConfigFileExists bool `json:"extsvcConfigFileExists"`

	ExtsvcConfigAllowEdits bool `json:"extsvcConfigAllowEdits"`

	RunningOnMacOS bool `json:"runningOnMacOS"`

	SvelteKit sveltekit.JSContext `json:"svelteKit"`

	// Bundle the Cody Pro configuration data that needs to be available on the frontend.
	FrontendCodyProConfig *FrontendCodyProConfig `json:"frontendCodyProConfig"`
}

// NewJSContextFromRequest populates a JSContext struct from the HTTP
// request.
func NewJSContextFromRequest(req *http.Request, db database.DB) JSContext {
	ctx := req.Context()
	a := sgactor.FromContext(ctx)

	headers := make(map[string]string)
	headers["x-sourcegraph-client"] = conf.ExternalURLParsed().String()
	headers["X-Requested-With"] = "Sourcegraph" // required for httpapi to use cookie auth

	// Propagate Cache-Control no-cache and max-age=0 directives
	// to the requests made by our client-side JavaScript. This is
	// not a perfect parser, but it catches the important cases.
	if cc := req.Header.Get("cache-control"); strings.Contains(cc, "no-cache") || strings.Contains(cc, "max-age=0") {
		headers["Cache-Control"] = "no-cache"
	}

	siteID := siteid.Get(db)

	// Show the site init screen?
	siteInitialized, err := db.GlobalState().SiteInitialized(ctx)
	needsSiteInit := err == nil && !siteInitialized

	// Auth providers
	authProviders := []authProviderInfo{} // Explicitly initialise array, otherwise it gets marshalled to null instead of []

	authzProviders, _, _, _ := authzproviders.ProvidersFromConfig(ctx, conf.Get(), db)
	for _, p := range providers.SortedProviders() {
		commonConfig := providers.GetAuthProviderCommon(p)
		if commonConfig.Hidden {
			continue
		}

		info := p.CachedInfo()
		if info == nil {
			continue
		}

		requiredForAuthz := slices.ContainsFunc(authzProviders, func(authzProvider authz.Provider) bool {
			return authzProvider.ServiceID() == info.ServiceID && authzProvider.ServiceType() == p.ConfigID().Type
		})

		providerInfo := authProviderInfo{
			IsBuiltin:         p.Config().Builtin != nil,
			NoSignIn:          commonConfig.NoSignIn,
			DisplayName:       commonConfig.DisplayName,
			DisplayPrefix:     commonConfig.DisplayPrefix,
			ServiceType:       p.ConfigID().Type,
			AuthenticationURL: info.AuthenticationURL,
			ServiceID:         info.ServiceID,
			ClientID:          info.ClientID,
			RequiredForAuthz:  requiredForAuthz,
		}

		authProviders = append(authProviders, providerInfo)
	}

	pp := conf.AuthPasswordPolicy()

	var authPasswordPolicy authPasswordPolicy
	authPasswordPolicy.Enabled = pp.Enabled
	authPasswordPolicy.NumberOfSpecialCharacters = pp.NumberOfSpecialCharacters
	authPasswordPolicy.RequireAtLeastOneNumber = pp.RequireAtLeastOneNumber
	authPasswordPolicy.RequireUpperAndLowerCase = pp.RequireUpperandLowerCase

	var sentryDSN *string
	siteConfig := conf.Get().SiteConfiguration

	if siteConfig.Log != nil && siteConfig.Log.Sentry != nil && siteConfig.Log.Sentry.Dsn != "" {
		sentryDSN = &siteConfig.Log.Sentry.Dsn
	}

	var openTelemetry *schema.OpenTelemetry
	if clientObservability := siteConfig.ObservabilityClient; clientObservability != nil {
		openTelemetry = clientObservability.OpenTelemetry
	}

	var user *types.User
	temporarySettings := "{}"
	if a.IsAuthenticated() {
		// Ignore err as we don't care if user does not exist
		user, _ = a.User(ctx, db.Users())
		if user != nil {
			if settings, err := db.TemporarySettings().GetTemporarySettings(ctx, user.ID); err == nil {
				temporarySettings = settings.Contents
			}
		}
	}

	siteResolver := graphqlbackend.NewSiteResolver(logger.Scoped("jscontext"), db)
	needsRepositoryConfiguration, err := siteResolver.NeedsRepositoryConfiguration(ctx)
	if err != nil {
		needsRepositoryConfiguration = false
	}

	extsvcConfigFileExists := envvar.ExtsvcConfigFile() != ""
	runningOnMacOS := runtime.GOOS == "darwin"

	accessTokenDefaultExpirationDays, accessTokenExpirationDaysOptions := conf.AccessTokensExpirationOptions()

	codyEnabled, _ := cody.IsCodyEnabled(ctx, db)

	isDotComMode := dotcom.SourcegraphDotComMode()

	licenseInfo, codeSearchLicensed, codyLicensed := licenseInfo()

	// 🚨 SECURITY: This struct is sent to all users regardless of whether or
	// not they are logged in, for example on an auth.public=false private
	// server. Including secret fields here is OK if it is based on the user's
	// authentication above, but do not include e.g. hard-coded secrets about
	// the server instance here as they would be sent to anonymous users.
	context := JSContext{
		ExternalURL:         conf.ExternalURLParsed().String(),
		XHRHeaders:          headers,
		UserAgentIsBot:      isBot(req.UserAgent()),
		AssetsRoot:          assetsutil.URL("").String(),
		Version:             version.Version(),
		IsAuthenticatedUser: a.IsAuthenticated(),
		CurrentUser:         createCurrentUser(ctx, user, db),
		TemporarySettings:   &TemporarySettings{GraphQLTypename: "TemporarySettings", Contents: temporarySettings},

		SentryDSN:                  sentryDSN,
		OpenTelemetry:              openTelemetry,
		RedirectUnsupportedBrowser: siteConfig.RedirectUnsupportedBrowser,
		Debug:                      env.InsecureDev,
		SiteID:                     siteID,

		SiteGQLID: string(graphqlbackend.SiteGQLID()),

		NeedsSiteInit:     needsSiteInit,
		EmailEnabled:      conf.CanSendEmail(),
		Site:              publicSiteConfiguration(),
		NeedServerRestart: globals.ConfigurationServerFrontendOnly.NeedServerRestart(),
		DeployType:        deploy.Type(),

		SourcegraphDotComMode: isDotComMode,

		// Experiments. We pass these through explicitly, so we can
		// do the default behavior only in Go land.
		AccessTokensAllow:                 conf.AccessTokensAllow(),
		AccessTokensAllowNoExpiration:     conf.AccessTokensAllowNoExpiration(),
		AccessTokensDefaultExpirationDays: accessTokenDefaultExpirationDays,
		AccessTokensExpirationDaysOptions: accessTokenExpirationDaysOptions,

		ResetPasswordEnabled: userpasswd.ResetPasswordEnabled(),

		AllowSignup: conf.AuthAllowSignup(),

		AuthMinPasswordLength: conf.AuthMinPasswordLength(),
		AuthPasswordPolicy:    authPasswordPolicy,

		AuthProviders:                  authProviders,
		AuthPrimaryLoginProvidersCount: conf.AuthPrimaryLoginProvidersCount(),

		AuthAccessRequest: conf.Get().AuthAccessRequest,

		Branding: conf.Branding(),

		BatchChangesEnabled:                batches.IsEnabled() && enterprise.BatchChangesEnabledForUser(ctx, db) == nil,
		BatchChangesDisableWebhooksWarning: conf.Get().BatchChangesDisableWebhooksWarning,
		BatchChangesWebhookLogsEnabled:     webhooks.LoggingEnabled(conf.Get()),

		CodyEnabledOnInstance:     conf.CodyEnabled(),
		CodyEnabledForCurrentUser: codyEnabled,
		CodyRequiresVerifiedEmail: siteResolver.RequiresVerifiedEmailForCody(ctx),

		CodeSearchEnabledOnInstance: codeSearchLicensed,
		ApplianceUpdateTarget:       conf.ApplianceUpdateTarget(),
		ApplianceMenuTarget:         conf.ApplianceMenuTarget(),

		ExecutorsEnabled:                               conf.ExecutorsEnabled(),
		CodeIntelAutoIndexingEnabled:                   conf.CodeIntelAutoIndexingEnabled(),
		CodeIntelAutoIndexingAllowGlobalPolicies:       conf.CodeIntelAutoIndexingAllowGlobalPolicies(),
		CodeIntelRankingDocumentReferenceCountsEnabled: conf.CodeIntelRankingDocumentReferenceCountsEnabled(),

		CodeInsightsEnabled: insights.IsEnabled(),

		// This used to be hardcoded configuration on the frontend.
		// https://sourcegraph.sourcegraph.com/github.com/sourcegraph/sourcegraph@ec5cc97a11c3f78743388b85b9ae0f1bc5d43932/-/blob/client/web/src/enterprise/EnterpriseWebApp.tsx?L63-71
		CodeIntelligenceEnabled:  true,
		SearchContextsEnabled:    searchcontexts.IsEnabled(),
		NotebooksEnabled:         notebooks.IsEnabled(),
		CodeMonitoringEnabled:    codemonitors.IsEnabled(),
		SearchAggregationEnabled: true,
		OwnEnabled:               own.IsEnabled(),
		SearchJobsEnabled:        exhaustive.IsEnabled(conf.Get()),

		ProductResearchPageEnabled: conf.ProductResearchPageEnabled(),

		ExperimentalFeatures: conf.ExperimentalFeatures(),

		LicenseInfo: licenseInfo,

		HashedLicenseKey: conf.HashedCurrentLicenseKeyForAnalytics(),

		OutboundRequestLogLimit: conf.Get().OutboundRequestLogLimit,

		DisableFeedbackSurvey: conf.Get().DisableFeedbackSurvey,

		NeedsRepositoryConfiguration: needsRepositoryConfiguration,

		ExtsvcConfigFileExists: extsvcConfigFileExists,

		ExtsvcConfigAllowEdits: envvar.ExtsvcConfigAllowEdits(),

		RunningOnMacOS: runningOnMacOS,

		SvelteKit: sveltekit.GetJSContext(req.Context()),
	}
	if dotcomConfig := conf.Get().Dotcom; dotcomConfig != nil {
		if codyProConfig := dotcomConfig.CodyProConfig; codyProConfig != nil {
			context.FrontendCodyProConfig = makeFrontendCodyProConfig(dotcomConfig.CodyProConfig)
		}
	}

	// If the license a Sourcegraph instance is running under does not support Code Search features
	// we force disable related features (executors, batch-changes, executors, code-insights).
	if !codeSearchLicensed {
		context.CodeSearchEnabledOnInstance = false
		context.BatchChangesEnabled = false
		context.CodeInsightsEnabled = false
		context.ExecutorsEnabled = false
		context.CodeMonitoringEnabled = false
		context.CodeIntelligenceEnabled = false
		context.SearchAggregationEnabled = false
		context.SearchContextsEnabled = false
		context.OwnEnabled = false
		context.NotebooksEnabled = false
		context.SearchJobsEnabled = false
	}

	// If the license a Sourcegraph instance is running under does not support Cody features,
	// we force disable related features.
	if !codyLicensed {
		context.CodyEnabledOnInstance = false
		context.CodyEnabledForCurrentUser = false
	}

	return context
}

// createCurrentUser creates CurrentUser object which contains of types.User
// properties along with some extra data such as user emails, organisations,
// session information, etc.
//
// We return a nil CurrentUser object on any error.
func createCurrentUser(ctx context.Context, user *types.User, db database.DB) *CurrentUser {
	if user == nil {
		return nil
	}

	userResolver := graphqlbackend.NewUserResolver(ctx, db, user)

	siteAdmin, err := userResolver.SiteAdmin()
	if err != nil {
		return nil
	}
	canAdminister, err := userResolver.ViewerCanAdminister()
	if err != nil {
		return nil
	}

	session, err := userResolver.Session(ctx)
	if err != nil && session == nil {
		return nil
	}

	hasVerifiedEmail, err := userResolver.HasVerifiedEmail(ctx)
	if err != nil {
		return nil
	}

	return &CurrentUser{
		GraphQLTypename:     "User",
		AvatarURL:           userResolver.AvatarURL(),
		Session:             &UserSession{session.CanSignOut()},
		DatabaseID:          userResolver.DatabaseID(),
		DisplayName:         derefString(userResolver.DisplayName()),
		Emails:              resolveUserEmails(ctx, userResolver),
		ID:                  userResolver.ID(),
		LatestSettings:      resolveLatestSettings(ctx, userResolver),
		Organizations:       resolveUserOrganizations(ctx, userResolver),
		SettingsURL:         derefString(userResolver.SettingsURL()),
		SiteAdmin:           siteAdmin,
		TosAccepted:         userResolver.TosAccepted(ctx),
		URL:                 userResolver.URL(),
		Username:            userResolver.Username(),
		ViewerCanAdminister: canAdminister,
		Permissions:         resolveUserPermissions(ctx, userResolver),
		HasVerifiedEmail:    hasVerifiedEmail,
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func resolveUserPermissions(ctx context.Context, userResolver *graphqlbackend.UserResolver) PermissionsConnection {
	connection := PermissionsConnection{
		GraphQLTypename: "PermissionConnection",
		Nodes:           []Permission{},
	}

	permissionResolver, err := userResolver.Permissions()
	if err != nil {
		return connection
	}

	nodes, err := permissionResolver.Nodes(ctx)
	if err != nil {
		return connection
	}

	for _, node := range nodes {
		connection.Nodes = append(connection.Nodes, Permission{
			GraphQLTypename: "Permission",
			ID:              node.ID(),
			DisplayName:     node.DisplayName(),
		})
	}

	return connection
}

func resolveUserOrganizations(ctx context.Context, user *graphqlbackend.UserResolver) *UserOrganizationsConnection {
	orgs, err := user.Organizations(ctx)
	if err != nil {
		return nil
	}
	userOrganizations := make([]UserOrganization, 0, len(orgs.Nodes()))
	for _, org := range orgs.Nodes() {
		userOrganizations = append(userOrganizations, UserOrganization{
			Typename:    "Org",
			ID:          org.ID(),
			Name:        org.Name(),
			URL:         org.URL(),
			SettingsURL: org.SettingsURL(),
		})
	}
	return &UserOrganizationsConnection{
		Typename: "OrgConnection",
		Nodes:    userOrganizations,
	}
}

func resolveUserEmails(ctx context.Context, user *graphqlbackend.UserResolver) []UserEmail {
	emails, err := user.Emails(ctx)
	if err != nil {
		return nil
	}

	userEmails := make([]UserEmail, 0, len(emails))

	for _, emailResolver := range emails {
		userEmail := UserEmail{
			Email:     emailResolver.Email(),
			IsPrimary: emailResolver.IsPrimary(),
			Verified:  emailResolver.Verified(),
		}
		userEmails = append(userEmails, userEmail)
	}

	return userEmails
}

func resolveLatestSettings(ctx context.Context, user *graphqlbackend.UserResolver) *UserLatestSettings {
	settings, err := user.LatestSettings(ctx)
	if err != nil {
		return nil
	}
	if settings == nil {
		return nil
	}
	return &UserLatestSettings{
		ID:       settings.ID(),
		Contents: settings.Contents(),
	}
}

// publicSiteConfiguration is the subset of the site.schema.json site
// configuration that is necessary for the web app and is not sensitive/secret.
func publicSiteConfiguration() schema.SiteConfiguration {
	c := conf.Get()
	return schema.SiteConfiguration{
		AuthPublic:                  c.AuthPublic,
		UpdateChannel:               conf.UpdateChannel(),
		AuthzEnforceForSiteAdmins:   c.AuthzEnforceForSiteAdmins,
		DisableNonCriticalTelemetry: c.DisableNonCriticalTelemetry,
	}
}

var isBotPat = lazyregexp.New(`(?i:googlecloudmonitoring|pingdom.com|go .* package http|sourcegraph e2etest|bot|crawl|slurp|spider|feed|rss|camo asset proxy|http-client|sourcegraph-client)`)

func isBot(userAgent string) bool {
	return isBotPat.MatchString(userAgent)
}

func licenseInfo() (info LicenseInfo, codeSearchLicensed, codyLicensed bool) {
	if !dotcom.SourcegraphDotComMode() {
		bcFeature := &licensing.FeatureBatchChanges{}
		if err := licensing.Check(bcFeature); err == nil {
			if bcFeature.Unrestricted {
				info.BatchChanges = &FeatureBatchChanges{
					Unrestricted: true,
					// Superceded by being unrestricted
					MaxNumChangesets: -1,
				}
			} else {
				max := int(bcFeature.MaxNumChangesets)
				info.BatchChanges = &FeatureBatchChanges{
					MaxNumChangesets: max,
				}
			}
		}
	}

	codeSearchLicensed = licensing.Check(licensing.FeatureCodeSearch) == nil
	codyLicensed = licensing.Check(licensing.FeatureCody) == nil

	return info, codeSearchLicensed, codyLicensed
}

func makeFrontendCodyProConfig(config *schema.CodyProConfig) *FrontendCodyProConfig {
	if config == nil {
		return nil
	}
	return &FrontendCodyProConfig{
		StripePublishableKey: config.StripePublishableKey,
		SscBaseUrl:           config.SscBaseUrl,
		UseEmbeddedUI:        config.UseEmbeddedUI,
	}
}
