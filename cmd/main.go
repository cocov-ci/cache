package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"

	"github.com/cocov-ci/cache/commands"
)

func envs(base string) []string {
	return []string{"COCOV_" + base, base}
}

func main() {
	fmt.Println("             .                     ")
	fmt.Println("             *                     ")
	fmt.Println("            :*                     ")
	fmt.Println("           ::@=-:  *= :            ")
	fmt.Println("          -* #--+@+%##@- :         ")
	fmt.Println("          +%-:#  .+*-*.=#*         ")
	fmt.Println("          .%+.=:    :  . #%-       ")
	fmt.Println("            -*:           #:       Cocov Cache")
	fmt.Println("                -:  +.  .=.        Copyright (c) 2022-2023 - The Cocov Authors")
	fmt.Println("             .**=       %%#=       Licensed under GPL-3.0")
	fmt.Println("            +*. =:   :-=+..        ")
	fmt.Println("          .#=    =#=-=%::*         ")
	fmt.Println("         -%:      =#. :+*=         ")
	fmt.Println("        +#         ##.  .          ")
	fmt.Println("       #*          :+@.            ")
	fmt.Println("     .%+          :% @+            ")
	fmt.Println("    .%=           -@:@*            ")

	app := cli.NewApp()
	app.Name = "cocov-worker"
	app.Usage = "Executes Cocov checks on commits"
	app.Version = "0.1"
	app.DefaultCommand = "run"
	app.Flags = []cli.Flag{
		&cli.StringFlag{Name: "redis-url", EnvVars: envs("REDIS_URL"), Required: true},
		&cli.StringFlag{Name: "storage-mode", EnvVars: envs("CACHE_STORAGE_MODE"), Required: true},
		&cli.StringFlag{Name: "local-storage-path", EnvVars: envs("CACHE_LOCAL_STORAGE_PATH"), Required: false},
		&cli.StringFlag{Name: "s3-bucket-name", EnvVars: envs("CACHE_S3_BUCKET_NAME"), Required: false},
		&cli.StringFlag{Name: "bind-address", EnvVars: envs("BIND_ADDRESS"), Required: false, Value: "0.0.0.0:5000"},
		&cli.Int64Flag{Name: "max-package-size-bytes", EnvVars: envs("MAX_PACKAGE_SIZE_BYTES"), Required: false, Value: 0},
	}
	app.Authors = []*cli.Author{
		{Name: "Victor \"Vito\" Gama", Email: "hey@vito.io"},
	}
	app.Copyright = "Copyright (c) 2022-2023 - The Cocov Authors"
	app.Commands = []*cli.Command{
		{
			Name:   "serve",
			Usage:  "Starts a server",
			Action: commands.Serve,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Println()
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
}
