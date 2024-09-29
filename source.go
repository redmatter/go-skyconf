package skyconf

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	ssmpkg "github.com/aws/aws-sdk-go-v2/service/ssm"
	"strings"
)

type ssmSource struct {
	ssm  *ssmpkg.Client
	path string
	id   string
}

// SSMSource creates a new SSM source.
func SSMSource(ssm *ssmpkg.Client, path string) Source {
	return SSMSourceWithID(ssm, path, "ssm")
}

// SSMSourceWithID creates a new SSM source with a custom ID.
func SSMSourceWithID(ssm *ssmpkg.Client, path, id string) Source {
	// ensure path ends with a slash
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}

	return &ssmSource{
		ssm:  ssm,
		path: path,
		id:   id,
	}
}

func (s *ssmSource) Source(ctx context.Context, keys []string) (values map[string]string, err error) {
	// Use GetParameters API to fetch the parameters
	input := &ssmpkg.GetParametersInput{
		Names:          keys,
		WithDecryption: aws.Bool(true),
	}

	var output *ssmpkg.GetParametersOutput
	output, err = s.ssm.GetParameters(ctx, input)
	if err != nil {
		err = fmt.Errorf("failed to get parameters: %w", err)
		return
	}

	// Map the parameters for easier access
	values = make(map[string]string, len(output.Parameters))
	for _, p := range output.Parameters {
		values[aws.ToString(p.Name)] = aws.ToString(p.Value)
	}

	return
}

func (s *ssmSource) ParameterName(parts []string) string {
	return makeParameterName(s.path, parts)
}

func makeParameterName(path string, parts []string) string {
	// Join the parts with a slash after converting them to snake case
	for i, part := range parts {
		parts[i] = ToSnakeCase(part)
	}

	return path + strings.Join(parts, "/")
}

func (s *ssmSource) ID() string {
	return s.id
}
