package completions

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sourcegraph/sourcegraph/internal/completions/types"
	"github.com/sourcegraph/sourcegraph/internal/database/dbmocks"

	modelconfigSDK "github.com/sourcegraph/sourcegraph/internal/modelconfig/types"
)

// getModelResult is a mock implementation of the getModelFn, one of the parameters
// to newCompletionsHandler.
type getModelResult struct {
	ModelRef modelconfigSDK.ModelRef
	Err      error
}

type mockGetModelFn struct {
	results []getModelResult
}

func (m *mockGetModelFn) PushResult(mref modelconfigSDK.ModelRef, err error) {
	result := getModelResult{
		ModelRef: mref,
		Err:      err,
	}
	m.results = append(m.results, result)
}

func (m *mockGetModelFn) ToFunc() getModelFn {
	return func(
		ctx context.Context, requestParams types.CodyCompletionRequestParameters, c *modelconfigSDK.ModelConfiguration) (
		modelconfigSDK.ModelRef, error) {
		if len(m.results) == 0 {
			panic("no call registered to getModels")
		}
		v := m.results[0]
		m.results = m.results[1:]
		return v.ModelRef, v.Err
	}
}

func TestGetCodeCompletionsModelFn(t *testing.T) {
	ctx := context.Background()
	getModelFn := getCodeCompletionModelFn()

	t.Run("ErrorUnsupportedModel", func(t *testing.T) {
		reqParams := types.CodyCompletionRequestParameters{
			CompletionRequestParameters: types.CompletionRequestParameters{
				Model: "model-the-user-requested",
			},
		}
		_, err := getModelFn(ctx, reqParams, nil /* modelconfigSDK.ModelConfiguration */)
		require.ErrorContains(t, err, "no configuration data supplied")

		_, err2 := getModelFn(ctx, reqParams, &modelconfigSDK.ModelConfiguration{})
		require.ErrorContains(t, err2, `unsupported code completion model "model-the-user-requested"`)
	})

	t.Run("OverrideSiteConfig", func(t *testing.T) {
		// Empty model config, except that it does contain the expected model.
		modelConfig := modelconfigSDK.ModelConfiguration{
			Models: []modelconfigSDK.Model{
				{ModelRef: "google::xxxx::some-other-model1"},
				{ModelRef: "google::xxxx::gemini-pro"},
				{ModelRef: "google::xxxx::some-other-model2"},
			},
		}
		reqParams := types.CodyCompletionRequestParameters{
			// BUG: This is inconsistent with how user-requested models work for "chats", which
			// totally ignore user preferences. Here we _always_ honor the user's preference.
			//
			// We should reject requests to use models the calling user cannot access, or are not
			// available to the current "Cody Pro Subscription" or "Cody Enterprise config".
			CompletionRequestParameters: types.CompletionRequestParameters{
				Model: "google/gemini-pro",
			},
		}
		gotMRef, err := getModelFn(ctx, reqParams, &modelConfig)
		require.NoError(t, err)
		assert.EqualValues(t, "google::xxxx::gemini-pro", gotMRef)
	})

	t.Run("Default", func(t *testing.T) {
		// For these tests, the Model field in the request body isn't set.
		// The default Code Completion model should be returned.
		t.Run("NoSiteConfig", func(t *testing.T) {
			reqParams := types.CodyCompletionRequestParameters{}
			_, err := getModelFn(ctx, reqParams, nil)
			assert.ErrorContains(t, err, "no configuration data supplied")
		})
		t.Run("WithSiteConfig", func(t *testing.T) {
			modelConfig := modelconfigSDK.ModelConfiguration{
				Models: []modelconfigSDK.Model{
					{ModelRef: "other-model-1"},
					{ModelRef: "other-model-2"},
					{ModelRef: "code-model-in-config"},
					{ModelRef: "other-model-3"},
				},
				DefaultModels: modelconfigSDK.DefaultModels{
					CodeCompletion: "code-model-in-config",
				},
			}

			reqParams := types.CodyCompletionRequestParameters{}
			model, err := getModelFn(ctx, reqParams, &modelConfig)
			require.NoError(t, err)
			assert.EqualValues(t, "code-model-in-config", model)
		})
	})
}

func TestGetChatModelFn(t *testing.T) {
	ctx := context.Background()
	mockDB := dbmocks.NewMockDB()

	t.Run("CodyEnterprise", func(t *testing.T) {
		t.Run("Chat", func(t *testing.T) {
			getModelFn := getChatModelFn(mockDB)
			modelConfig := modelconfigSDK.ModelConfiguration{
				Models: []modelconfigSDK.Model{
					{ModelRef: "some-other-model"},
					{ModelRef: "model-the-user-requested"},
				},
				DefaultModels: modelconfigSDK.DefaultModels{
					Chat: "default-chat-model",
				},
			}

			t.Run("Found", func(t *testing.T) {
				reqParams := types.CodyCompletionRequestParameters{
					CompletionRequestParameters: types.CompletionRequestParameters{
						Model: "model-the-user-requested",
					},
				}
				model, err := getModelFn(ctx, reqParams, &modelConfig)

				require.NoError(t, err)
				assert.EqualValues(t, "model-the-user-requested", model)
			})

			// User requests to use an LLM model not supported by the backend.
			t.Run("NotFound", func(t *testing.T) {
				reqParams := types.CodyCompletionRequestParameters{
					CompletionRequestParameters: types.CompletionRequestParameters{
						Model: "some-model-not-in-config",
					},
				}
				_, err := getModelFn(ctx, reqParams, &modelConfig)
				require.ErrorContains(t, err, `unsupported code completion model "some-model-not-in-config"`)
			})
		})

		t.Run("FastChat", func(t *testing.T) {
			getModelFn := getChatModelFn(mockDB)
			modelConfig := modelconfigSDK.ModelConfiguration{
				Models: []modelconfigSDK.Model{
					{ModelRef: "some-other-model"},
					{ModelRef: "model-the-user-requested"},
				},
				DefaultModels: modelconfigSDK.DefaultModels{
					Chat:     "default-chat-model",
					FastChat: "default-fastchat-model",
				},
			}

			reqParams := types.CodyCompletionRequestParameters{
				CompletionRequestParameters: types.CompletionRequestParameters{
					Model: "model-the-user-requested",
				},
				// .. but again, for "fast" chats.
				Fast: true,
			}
			model, err := getModelFn(ctx, reqParams, &modelConfig)

			require.NoError(t, err)
			// We use the FastChat model, regardless of what the user requested.
			assert.EqualValues(t, "default-fastchat-model", model)
		})
	})

	// TODO(PRIME-283): As part of enabling model selection for Cody Enterprise users,
	// add more tests for the Cody Pro path as well. Where we only allow certain models
	// based on the calling user's subscription status, etc.
}
