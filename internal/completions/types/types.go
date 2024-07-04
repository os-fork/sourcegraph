package types

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/sourcegraph/log"

	"github.com/sourcegraph/sourcegraph/lib/errors"

	modelconfig "github.com/sourcegraph/sourcegraph/internal/modelconfig/types"
)

const HUMAN_MESSAGE_SPEAKER = "human"
const ASSISTANT_MESSAGE_SPEAKER = "assistant"
const SYSTEM_MESSAGE_SPEAKER = "system"

type Message struct {
	Speaker string `json:"speaker"`
	Text    string `json:"text"`
}

func (m Message) IsValidSpeaker() bool {
	return m.Speaker == HUMAN_MESSAGE_SPEAKER || m.Speaker == ASSISTANT_MESSAGE_SPEAKER
}

func (m Message) GetPrompt(humanPromptPrefix, assistantPromptPrefix string) (string, error) {
	var prefix string
	switch m.Speaker {
	case HUMAN_MESSAGE_SPEAKER:
		prefix = humanPromptPrefix
	case ASSISTANT_MESSAGE_SPEAKER:
		prefix = assistantPromptPrefix
	default:
		return "", errors.Newf("expected message speaker to be 'human' or 'assistant', got %s", m.Speaker)
	}

	if len(m.Text) == 0 {
		// Important: no trailing space (affects output quality)
		return prefix, nil
	}
	return fmt.Sprintf("%s %s", prefix, m.Text), nil
}

type CodyCompletionRequestParameters struct {
	CompletionRequestParameters

	// When Fast is true, then it is used as a hint to prefer a model
	// that is faster (but probably "dumber").
	Fast bool
}

// TaintedModelRef is a ModelRef that came from the Cody client, and therefore has no
// guarantee if it is in the older format of "PROVIDER/MODEL" or the newer ModelRef
// format "PROVIDER::API-VERSION::MODEL".
//
// You MUST NOT blindly cast this to a modelconfigSDK.ModelRef, as it will certainly
// cause failures at runtime. Instead, it must be inspected and carefully parsed.
type TaintedModelRef string

type CompletionRequestParameters struct {
	// This should NOT be used. It is here to be part of the incomming HTTP request,
	// but the actual LLM model that should be used when _serving_ the request is defined
	// by the ModelConfigInfo.Model, and not what the user specified.
	Model TaintedModelRef

	Messages          []Message `json:"messages"`
	MaxTokensToSample int       `json:"maxTokensToSample,omitempty"`
	Temperature       float32   `json:"temperature,omitempty"`
	StopSequences     []string  `json:"stopSequences,omitempty"`
	TopK              int       `json:"topK,omitempty"`
	TopP              float32   `json:"topP,omitempty"`
	Stream            *bool     `json:"stream,omitempty"`
	Logprobs          *uint8    `json:"logprobs"`

	// Prompt exists only for backwards compatibility. Do not use it in new
	// implementations. It will be removed once we are reasonably sure 99%
	// of VSCode extension installations are upgraded to a new Cody version.
	Prompt string `json:"prompt"`
}

// IsStream returns whether a streaming response is requested. For backwards
// compatibility reasons, we are using a pointer to a bool instead of a bool
// to default to true in case the value is not explicity provided.
func (p CompletionRequestParameters) IsStream(feature CompletionsFeature) bool {
	if p.Stream == nil {
		return defaultStreamMode(feature)
	}
	return *p.Stream
}

func defaultStreamMode(feature CompletionsFeature) bool {
	switch feature {
	case CompletionsFeatureChat:
		return true
	case CompletionsFeatureCode:
		return false
	default:
		// Safeguard, should be never reached.
		return true
	}
}

func (p *CompletionRequestParameters) Attrs(modelName string, feature CompletionsFeature) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Int("promptLength", len(p.Prompt)),
		attribute.Int("numMessages", len(p.Messages)),
		attribute.Int("maxTokensToSample", p.MaxTokensToSample),
		attribute.Float64("temperature", float64(p.Temperature)),
		attribute.Int("topK", p.TopK),
		attribute.Float64("topP", float64(p.TopP)),
		attribute.String("model", modelName),
		attribute.Bool("stream", p.IsStream(feature)),
	}
}

type CompletionResponse struct {
	Completion string    `json:"completion"`
	StopReason string    `json:"stopReason"`
	Logprobs   *Logprobs `json:"logprobs,omitempty"`
}

type Logprobs struct {
	Tokens        []string             `json:"tokens"`
	TokenLogprobs []float32            `json:"token_logprobs"`
	TopLogprobs   []map[string]float32 `json:"top_logprobs"`
	TextOffset    []int32              `json:"text_offset"`
}

// Append concatenates the additional logprobs to the original ones
// and returns a reference to the mutated original logprobs. Note
// this mutates the receiver. If the receiver is nil, a shallow
// copy of additional is returned.
//
// Intended usage: original = original.Append(additional)
func (original *Logprobs) Append(additional *Logprobs) *Logprobs {
	if original == nil {
		if additional == nil {
			return nil
		} else {
			newLogprobs := *additional
			return &newLogprobs
		}
	}
	if additional == nil {
		return original
	}

	original.Tokens = append(original.Tokens, additional.Tokens...)
	original.TokenLogprobs = append(original.TokenLogprobs, additional.TokenLogprobs...)
	original.TopLogprobs = append(original.TopLogprobs, additional.TopLogprobs...)
	original.TextOffset = append(original.TextOffset, additional.TextOffset...)
	return original
}

type SendCompletionEvent func(event CompletionResponse) error

type CompletionsFeature string

const (
	CompletionsFeatureChat CompletionsFeature = "chat_completions"
	CompletionsFeatureCode CompletionsFeature = "code_completions"
)

func (b CompletionsFeature) IsValid() bool {
	switch b {
	case CompletionsFeatureChat,
		CompletionsFeatureCode:
		return true
	}
	return false
}

// ID returns a numeric ID representing the feature for analytics purposes.
func (b CompletionsFeature) ID() int {
	switch b {
	case CompletionsFeatureChat:
		return 1
	case CompletionsFeatureCode:
		return 2
	default:
		return -1
	}
}

type CompletionsVersion int

const (
	CompletionsVersionLegacy CompletionsVersion = 0
	CompletionsV1            CompletionsVersion = 1
)

// CodyClientName represents the name of a client in URL query parameters.
type CodyClientName string

const (
	CodyClientWeb       CodyClientName = "web"
	CodyClientVscode    CodyClientName = "vscode"
	CodyClientJetbrains CodyClientName = "jetbrains"
)

// ModelConfigInfo is all the configuration information about the LLM Model and
// the Provider we are using to resolve the request.
type ModelConfigInfo struct {
	Provider modelconfig.Provider
	Model    modelconfig.Model

	// CodyProUserAccessToken is an awkward hack for the asymmetry between Cody Enterprise
	// and Cody Pro. For Cody Enterprise, requests are sent to Cody Gateway using the
	// Sourcegraph instance's access token derived from their license key. (i.e. all the
	// server-side configuration data found in `Provider.ServerSideConfig.SourcegraphProviderConfig`.
	//
	// Leave as `nil` for Cody Enterprise requests, which will then use the access token
	// from the Provider.ServerSideConfig.SourcegraphProviderConfig.
	//
	// Cody Pro users are authenticated on Cody Gateway by their dotcom access token. So
	// it doesn't make sense to store that in the Provider's server-side config, as it is
	// instead bound to this particular HTTP request.
	//
	// In the future, we'll be able to rectify this by having Cody Free/Cody Pro users authenticate
	// via a shared access token bound to the "Cody Pro Team" / "Sourcegraph Organization".
	CodyProUserAccessToken *string
}

type CompletionRequest struct {
	Feature         CompletionsFeature
	ModelConfigInfo ModelConfigInfo
	Parameters      CompletionRequestParameters
	Version         CompletionsVersion
}

type CompletionsClient interface {
	// Stream executions a completions request, streaming results to the callback.
	// Callers should check for ErrStatusNotOK and handle the error appropriately.
	Stream(context.Context, log.Logger, CompletionRequest, SendCompletionEvent) error
	// Complete executions a completions request until done. Callers should check
	// for ErrStatusNotOK and handle the error appropriately.
	Complete(context.Context, log.Logger, CompletionRequest) (*CompletionResponse, error)
}

func ConvertFromLegacyMessages(messages []Message) []Message {
	filteredMessages := make([]Message, 0)
	skipNext := false
	for i, message := range messages {
		if skipNext {
			skipNext = false
			continue
		}

		// 1. If the first message is "system prompt like" convert it to an actual system prompt
		//
		// Note: The prefix we scan for here is used in the current chat prompts for VS Code and the
		//       old Web UI prompt.
		if i == 0 && strings.HasPrefix(message.Text, "You are Cody, an AI") {
			message.Speaker = SYSTEM_MESSAGE_SPEAKER
			skipNext = true
		}

		if i == len(messages)-1 && message.Speaker == ASSISTANT_MESSAGE_SPEAKER {
			// 2. If the last message is from an `assistant` with no or empty `text`, omit it
			if message.Text == "" {
				continue
			}

			// 3. Final assistant content cannot end with trailing whitespace
			message.Text = strings.TrimRight(message.Text, " \t\n\r")

		}

		// 4. If there is any assistant message in the middle of the messages without a `text`, omit
		//    both the empty assistant message as well as the unanswered question from the `user`

		// Don't apply this to the human message before the last message (it should always be included)
		if i >= len(messages)-2 {
			filteredMessages = append(filteredMessages, message)
			continue
		}
		// If the next message is an assistant message with no or empty `content`, omit the current and
		// the next one
		nextMessage := messages[i+1]
		if (nextMessage.Speaker == ASSISTANT_MESSAGE_SPEAKER && nextMessage.Text == "") ||
			(message.Speaker == ASSISTANT_MESSAGE_SPEAKER && message.Text == "") {
			continue
		}
		filteredMessages = append(filteredMessages, message)
	}

	return filteredMessages
}
