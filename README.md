# Ský Config for Go

Ský Config is a configuration library for Go. It can be used to load configuration
values from cloud based parameter stores such as AWS SSM Parameter Store.

> ### Ský, why?
> 
> Ský /_skiː_/ is the [Danish word for cloud](https://en.bab.la/dictionary/english-danish/cloud), 
> which is where the configuration values are stored.

## How to use

See the [example](example_test.go) for how to use the library.

## Working with SSM Parameter Hierarchies

AWS Systems Manager (SSM) Parameter Store allows organizing parameters into
hierarchies using forward slashes (`/`) in parameter names. This is a powerful
feature for managing configuration for different environments (e.g., `/dev`,
`/staging`, `/prod`) or different components of an application.

For example, you could structure your parameters like this:

```
/my-project/common/database_url
/my-project/common/database_port
/my-project/my-component/service_endpoint
/my-project/my-component/timeout
```

`go-skyconf` makes it easy to work with such hierarchical configurations. When
you process a struct, `go-skyconf` will fetch parameters from the path prefix
you provide.

### Overriding the root path with `source`

A component often needs its own configuration as well as shared configuration
from a higher level in the hierarchy. `go-skyconf` supports this by using the
`source` tag on a struct field to refer to a named `sky.Source`.

First, you define named sources when you initialize `go-skyconf`. For example, you might have a "project" source for shared parameters and an "app" source for application-specific parameters.

```go
// In your setup code:
projectSource := sky.SSMSourceWithID(ssmClient, "/my-project/", "project")
appSource := sky.SSMSourceWithID(ssmClient, "/my-project/apps/my-app/", "app")

// ... then later when parsing configuration
sky.Parse(ctx, &cfg, false, projectSource, appSource)
```

Then, in your configuration struct, you can use the `source` tag to specify which named source to use for a particular field or nested struct.

```go
// Config contains settings for the application.
type Config struct {
	// This struct will be populated from the "project" source,
	// using the path "/my-project/".
	Database sqldb.Config `sky:"database/main_db,source:project"`

	// This struct will be populated from the "app" source,
	// using the path "/my-project/apps/my-app/".
	Finder FinderConfig `sky:",flatten,source:app"`

	// This field will be populated from the first source that contains it,
	// as no source is specified.
	LogLevel string `conf:"default:info"`
}
```

When `sky.Parse` is called on a `Config` struct:
1. It will populate the `Database` field by looking for parameters under the path `/my-project/database/main_db/` in SSM, because it's using the `project` source.
2. It will populate the fields of the `Finder` struct by looking for parameters under `/my-project/apps/my-app/`, because it's using the `app` source.
3. It will populate `LogLevel` from any of the provided sources, or use the default.

This allows different parts of your application to be configured from different parameter hierarchies, promoting separation of concerns and reusability of configuration.

For a more detailed example of using multiple sources, see the [multi-source example](example_test.go) (`Example_multipleSources`).

## Acknowledgements

This library is inspired by the [ardanlabs/conf](https://github.com/ardanlabs/conf) library by Ardan Labs.

## License

This project is licensed under the Apache License v2.0 - see the [LICENSE](LICENSE) file for details.

## Copyright

Copyright 2024, Red Matter Ltd.
