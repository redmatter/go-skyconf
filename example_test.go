package skyconf_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	sky "github.com/redmatter/go-skyconf"
)

// Example demonstrates how to use go-skyconf to parse configuration from AWS SSM.
//
// This example assumes you have a working AWS session and SSM client.
// It also assumes that the following parameters exist in AWS SSM under the path /myapplication/:
//   - app/name:    "MyApp"
//   - app/version: "1.2.3"
//   - db/host:     "localhost"
//   - db/port:     "5432"
//   - log_level:   "info"
func Example() {
	// 1. Define your configuration struct.
	// Use struct tags to control how fields are populated from SSM.
	type Config struct {
		App struct {
			Name    string `sky:"name"`
			Version string `sky:"version"`
		} `sky:"app"`
		Database struct {
			Host string `sky:"host"`
			Port int    `sky:"port"`
		} `sky:"db"`
		LogLevel string `sky:"log_level,optional"`
	}

	// 2. Set up a real SSM client from the AWS SDK.
	// In a real application, you would load the default AWS config.
	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Printf("unable to load SDK config, %v; skipping example", err)
		return
	}
	ssmClient := ssm.NewFromConfig(awsCfg)

	// 3. Define the SSM source for your configuration.
	// This tells go-skyconf where to look for parameters.
	ssmSource := sky.SSMSource(ssmClient, "/myapplication/")

	// 4. Create an instance of your config struct and parse the configuration.
	var cfg Config
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = sky.Parse(ctx, &cfg, false, ssmSource)
	if err != nil {
		// This will fail if the parameters don't exist in SSM.
		log.Printf("failed to parse configuration: %v; skipping example", err)
		return
	}

	// 5. Your config struct is now populated.
	fmt.Printf("AppName: %s\n", cfg.App.Name)
	fmt.Printf("AppVersion: %s\n", cfg.App.Version)
	fmt.Printf("DB Host: %s\n", cfg.Database.Host)
	fmt.Printf("DB Port: %d\n", cfg.Database.Port)
	fmt.Printf("Log Level: %s\n", cfg.LogLevel)

	// Unordered output:
	// AppName: MyApp
	// AppVersion: 1.2.3
	// DB Host: localhost
	// DB Port: 5432
	// Log Level: info
}

// ExampleParse demonstrates how to use the Parse function to parse configuration from AWS SSM.
func ExampleParse() {
	// This example is for documentation purposes and is not runnable as a standalone test
	// as it requires a pre-configured SSM client and parameters in AWS SSM.
	// For a more complete example, see Example().

	// Initialise SSM client using aws-sdk-go-v2
	var ssmClient *ssm.Client

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
	_, err := sky.Parse(ctx, &cfg, false, sky.SSMSource(ssmClient, os.Getenv("APP_SSM_PATH")+"database/"))
	if err != nil {
		log.Fatalln("failed to parse configuration:", err)
	}

	// Use the configuration
}

func ExampleRefresher() {
	// This example is for documentation purposes and is not runnable as a standalone test
	// as it requires a pre-configured SSM client and parameters in AWS SSM.
	// For a more complete example, see Example().

	// Initialise SSM client using aws-sdk-go-v2
	var ssmClient *ssm.Client

	// Create a timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

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
	refresher, err := sky.Parse(ctx, &cfg, false, sky.SSMSource(ssmClient, os.Getenv("APP_SSM_PATH")+"database/"))
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

// Example_multipleSources demonstrates how to use go-skyconf to parse configuration from multiple hierarchical sources in AWS SSM.
//
// This example assumes a working AWS session and that the following parameters exist in AWS SSM:
//
// Project-level parameters under /my-project/:
//   - db/host:     "project-db.example.com"
//   - db/port:     "5432"
//
// App-level parameters under /my-project/apps/my-app/:
//   - log_level:   "debug"
//   - app_name:    "MyAwesomeApp"
func Example_multipleSources() {
	// 1. Define your configuration struct with source tags.
	type Config struct {
		Database struct {
			Host string `sky:"host"`
			Port int    `sky:"port"`
		} `sky:"db,source:project"` // From project source
		App struct {
			LogLevel string `sky:"log_level"`
		} `sky:",flatten,source:app"` // From app source
		AppName string `sky:"app_name,source:app"` // From app source
	}

	// 2. Set up a real SSM client.
	awsCfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Printf("unable to load SDK config, %v; skipping example", err)
		return
	}
	ssmClient := ssm.NewFromConfig(awsCfg)

	// 3. Define multiple SSM sources with unique IDs.
	projectSource := sky.SSMSourceWithID(ssmClient, "/my-project/", "project")
	appSource := sky.SSMSourceWithID(ssmClient, "/my-project/apps/my-app/", "app")

	// 4. Parse the configuration using both sources.
	var cfg Config
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = sky.Parse(ctx, &cfg, false, projectSource, appSource)
	if err != nil {
		log.Printf("failed to parse configuration: %v; skipping example", err)
		return
	}

	// 5. Your config struct is now populated from multiple sources.
	fmt.Printf("DB Host: %s\n", cfg.Database.Host)
	fmt.Printf("DB Port: %d\n", cfg.Database.Port)
	fmt.Printf("Log Level: %s\n", cfg.App.LogLevel)
	fmt.Printf("AppName: %s\n", cfg.AppName)

	// Unordered output:
	// DB Host: project-db.example.com
	// DB Port: 5432
	// Log Level: debug
	// AppName: MyAwesomeApp
}
