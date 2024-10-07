package skyconf

import (
	"context"
	ssmpkg "github.com/aws/aws-sdk-go-v2/service/ssm"
	"log"
	"os"
	"time"
)

// ExampleParse demonstrates how to use the Parse function to parse configuration from AWS SSM.
func ExampleParse() {
	// Initialise SSM client using aws-sdk-go-v2
	var ssm *ssmpkg.Client

	// Create a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := struct {
		LogLevel string
		DB       struct {
			Host     string `sky:"host"`
			Port     int    `sky:"port"`
			Username string `sky:"username"`
			Password string `sky:"password"`
		} `sky:",flatten"`
	}{}

	// Parse database configuration from SSM
	_, err := Parse(ctx, &cfg, false, SSMSource(ssm, os.Getenv("APP_SSM_PATH")+"database/"))
	if err != nil {
		log.Fatalln("failed to parse configuration:", err)
	}

	// Use the configuration
}
