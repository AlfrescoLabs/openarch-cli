package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AlfrescoLabs/openarch-cli/internal/demo"
	"github.com/AlfrescoLabs/openarch-cli/internal/models"
	"github.com/AlfrescoLabs/openarch-cli/internal/output"
)

// modelFlags are the flags that pick the embedding model.
type modelFlags struct {
	model string
	url   string
}

func (m *modelFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&m.model, "model", models.EmbedModel(), "embedding model on Docker Model Runner (1024 dimensions or fewer); default from OPENARCH_EMBED_MODEL when set")
	cmd.Flags().StringVar(&m.url, "model-url", models.URL(), "OpenAI-compatible API of Docker Model Runner; default from OPENARCH_MODEL_URL when set")
}

// embedder returns a function that makes vectors with the model, and turns the
// usual failures into errors with a hint.
func (m *modelFlags) embedder() demo.Embedder {
	c := models.NewClient(m.url)
	return func(ctx context.Context, texts []string) ([][]float32, error) {
		v, err := c.Embed(ctx, m.model, texts)
		var merr *models.ModelError
		switch {
		case errors.Is(err, models.ErrUnreachable):
			return nil, output.NewError(output.ExitPrerequisite, err.Error(),
				"Start Docker Model Runner with host TCP access: docker desktop enable model-runner --tcp 12434. Or set --model-url.")
		case errors.As(err, &merr) && merr.Status == http.StatusNotFound:
			return nil, output.NewError(output.ExitPrerequisite, err.Error(), fmt.Sprintf("Pull the model: docker model pull %s", m.model))
		case errors.As(err, &merr) && strings.Contains(merr.Body, "physical batch size"):
			// The runner's default batch (512 tokens) is smaller than the input.
			return nil, output.NewError(output.ExitPrerequisite, err.Error(), fmt.Sprintf(
				"Give the model a larger context and batch: docker model configure --mode embedding --context-size 8192 %s -- --batch-size 8192 --ubatch-size 8192", m.model))
		}
		return v, err
	}
}
