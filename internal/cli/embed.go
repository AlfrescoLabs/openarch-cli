package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"

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
	cmd.Flags().StringVar(&m.model, "model", models.DefaultEmbedModel, "embedding model on Docker Model Runner (1024 dimensions or fewer)")
	cmd.Flags().StringVar(&m.url, "model-url", models.DefaultURL, "OpenAI-compatible API of Docker Model Runner")
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
		}
		return v, err
	}
}
