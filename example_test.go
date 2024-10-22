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

func ExampleRefresher_Refresh() {
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
		EnabledProductIDs string `sky:"enabled_list,refresh=5m,id=enabled_product_list"`
	}{}

	// Parse database configuration from SSM
	refresher, err := Parse(ctx, &cfg, false, SSMSource(ssm, os.Getenv("APP_SSM_PATH")+"database/"))
	if err != nil {
		log.Fatalln("failed to parse configuration:", err)
	}

	// Refresh the configuration according to the timings specified in the struct tags.
	// In this case, the `enabled_list` field will be refreshed every 5 minutes.
	updates := refresher.Refresh(ctx, func(err error) {
		if err != nil {
			log.Println("error refreshing configuration:", err)
		}
	})

	go func() {
		for update := range updates {
			log.Println("updated field:", update)
			switch update {
			case "enabled_product_list": // The ID specified in the struct tag
				// Do something with the updated field
			default:
				// This should never happen unless a new field is added.
				log.Fatalln("unknown field updated:", update)
			}
		}
	}()

	// Use the configuration

	// When done, cancel the context to stop the refresh
	cancel()
}
