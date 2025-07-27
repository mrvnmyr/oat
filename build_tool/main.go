package main

// This is a simple binary that builds the actual project.
// You could do the same thing with shell scripts, but we want to be
// cross-platform and not require much other than 'go' and 'git'.

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

const CONFIG_FILE_NAME = "build-tool-config.json"

var (
	buildHookPrePath  string
	buildHookPostPath string
)

func init() {
	scriptExt := "sh"
	if runtime.GOOS == "windows" {
		scriptExt = "bat"
	}

	buildHookPrePath = fmt.Sprintf("build-hook-pre.%s", scriptExt)
	buildHookPostPath = fmt.Sprintf("build-hook-post.%s", scriptExt)
}

var (
	flagDebug       = false
	flagOnlyCurrent = false
	flagNoGoGet     = false
	configPath      = ""
	config          BuildConfig
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func debugf(fmtStr string, v ...any) {
	if flagDebug {
		fmt.Printf(fmtStr, v...)
	}
}

type BuildConfig struct {
	Env       map[string]string `json:"env"`
	Platforms [][]string        `json:"platforms"`
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false // file doesn't exist or other error
	}
	mode := info.Mode()
	// Check if it's a regular file and executable by **someone**
	return mode.IsRegular() && (mode&0111 != 0)
}

func run(args []string, envMap map[string]string) {
	cmd := exec.Command(args[0], args[1:]...)

	// Set env vars: inherit, then override/add entry.Env
	env := os.Environ()
	if envMap != nil {
		for k, v := range envMap {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	cmd.Env = env

	if flagDebug {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	debugf("Running '%s'...\n", args)

	err := cmd.Run()
	check(err)
}

func findUpwards(filename string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		// Construct full path to file in this directory
		fullPath := filepath.Join(dir, filename)
		if _, err := os.Stat(fullPath); err == nil {
			return dir, nil // Found the file
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached the root directory, stop
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("file %s not found in any parent directory", filename)
}

func main() {
	{ // parse CLI flags
		flag.BoolVar(&flagDebug, "d", false, "Enable debug mode")
		flag.BoolVar(&flagDebug, "debug", false, "Enable debug mode (same as -d)")
		flag.BoolVar(&flagOnlyCurrent, "oc", false, "Build only for current GOOS/GOARCH (same as -oc)")
		flag.BoolVar(&flagOnlyCurrent, "only-current", false, "Build only for current GOOS/GOARCH")
		flag.BoolVar(&flagNoGoGet, "ngg", false, "Don't run 'go get' before building")
		flag.BoolVar(&flagNoGoGet, "no-go-get", false, "Don't run 'go get' before building (same as -ngg)")

		// Parse flags
		flag.Parse()
	}

	{ // cd to project root
		dir, err := findUpwards(CONFIG_FILE_NAME)
		check(err)

		err = os.Chdir(dir)
		check(err)

		cwd, err := os.Getwd()
		check(err)

		configPath = filepath.Join(dir, CONFIG_FILE_NAME)
		buildHookPrePath = filepath.Join(dir, buildHookPrePath)
		buildHookPostPath = filepath.Join(dir, buildHookPostPath)

		debugf("Current directory: %s\n", cwd)
		debugf("Config Path: %s\n", configPath)
		debugf("Build Hook Pre Path: %s\n", buildHookPrePath)
		debugf("Build Hook Post Path: %s\n", buildHookPostPath)
	}

	{ // parse 'build-oat.json'
		contents, err := os.ReadFile(configPath)
		check(err)

		err = json.Unmarshal(contents, &config)
		if err != nil {
			panic(err)
		}

		debugf("Config: %v\n", config)
	}

	// RunEntry describes a single process to launch
	type RunEntry struct {
		Args []string
		Env  map[string]string
	}

	var entries []RunEntry

	if !flagNoGoGet { // 'run go get' first
		run([]string{"go", "get"}, nil)
	}

	if !flagOnlyCurrent { // add all GOOS/GOARCH combinations from the config
		for _, triplet := range config.Platforms {
			goos := triplet[0]
			goarch := triplet[1]
			binExtension := triplet[2]

			fileSuffix := fmt.Sprintf("%s_%s%s", goos, goarch, binExtension)
			env := map[string]string{
				"GOOS":   goos,
				"GOARCH": goarch,
			}

			// spread config.Env into env
			for k, v := range config.Env {
				env[k] = v
			}

			fileName := fmt.Sprintf("oat_%s", fileSuffix)

			// append
			entries = append(entries, RunEntry{
				Args: []string{
					"go",
					"build",
					"-o",
					fmt.Sprintf("./bin/%s", fileName),
				},
				Env: env,
			})
		}
	}

	{ // add current GOOS/GOARCH
		entries = append(entries, RunEntry{
			Args: []string{
				"go",
				"build",
			},
			Env: config.Env,
		})
	}

	{ // run it all
		debugf("Building...\n")

		// Result holds the outcome of running a RunEntry
		type Result struct {
			Entry    RunEntry
			Stdout   string
			Stderr   string
			ExitCode int
			Err      error
		}

		runEntry := func(entry RunEntry) Result {
			if len(entry.Args) == 0 {
				return Result{Entry: entry, Err: fmt.Errorf("no command specified")}
			}

			cmd := exec.Command(entry.Args[0], entry.Args[1:]...)

			// Set env vars: inherit, then override/add entry.Env
			env := os.Environ()
			for k, v := range entry.Env {
				env = append(env, fmt.Sprintf("%s=%s", k, v))
			}
			cmd.Env = env

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			err := cmd.Run()
			exitCode := 0
			if err != nil {
				// Extract exit code if possible
				if exitErr, ok := err.(*exec.ExitError); ok {
					exitCode = exitErr.ExitCode()
				} else {
					exitCode = -1
				}
			} else if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}

			return Result{
				Entry:    entry,
				Stdout:   stdout.String(),
				Stderr:   stderr.String(),
				ExitCode: exitCode,
				Err:      err,
			}
		}

		{ // optionally run 'build-hook-pre' if existing
			if isExecutable(buildHookPrePath) {
				run([]string{buildHookPrePath}, nil)
			}
		}

		{
			var (
				numWorkers = runtime.NumCPU()
				jobs       = make(chan RunEntry)
				results    = make(chan Result, len(entries))
			)

			{ // Run all Entries in parallel
				var wg sync.WaitGroup

				// Start workers
				for i := 0; i < numWorkers; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						for entry := range jobs {
							results <- runEntry(entry)
						}
					}()
				}

				// Send jobs
				go func() {
					for _, entry := range entries {
						jobs <- entry
					}
					close(jobs)
				}()

				// Wait for all workers to finish
				go func() {
					wg.Wait()
					close(results)
				}()
			}

			{ // Print the results
				var sortedResults []Result
				{ // sort results according to string representation of result.Entry.Args - as we run it all in parallel which ofc mixes up "insertion order"
					for result := range results {
						sortedResults = append(sortedResults, result)
					}

					sort.Slice(sortedResults, func(i, j int) bool {
						return fmt.Sprintf("%v", sortedResults[i].Entry.Args) < fmt.Sprintf("%v", sortedResults[j].Entry.Args)
					})
				}

				var failures []Result
				for _, result := range sortedResults {
					if flagDebug {
						// debugf("---\nCommand: %v\nEnv: %v\nExit code: %d\nStdout: %sStderr: %s\n", result.Entry.Args, result.Entry.Env, result.ExitCode, result.Stdout, result.Stderr)
						debugf("---\nCommand: %v\nEnv: %v\n", result.Entry.Args, result.Entry.Env)
						if result.ExitCode != 0 {
							debugf("Exit Code: %d\n", result.ExitCode)
						}
						if result.Stdout != "" {
							debugf("Stdout: %s\n", result.Stdout)
						}
						if result.Stderr != "" {
							debugf("Stderr: %s\n", result.Stderr)
						}
					}
					if result.Err != nil || result.ExitCode != 0 {
						failures = append(failures, result)
					}
				}

				if len(failures) > 0 {
					fmt.Fprintf(os.Stderr, "XXX : Failures:\n")
					for _, fail := range failures {
						fmt.Fprintf(os.Stderr, "Command: %v\nExit code: %d\nStdout: %sStderr: %sError: %v\n---\n",
							fail.Entry.Args, fail.ExitCode, fail.Stdout, fail.Stderr, fail.Err)
					}
					os.Exit(1)
				}

				if flagOnlyCurrent {
					debugf("\nAll builds succeeded. (Only Current GOOS/GOARCH)\n")
				} else {
					debugf("\nAll builds succeeded.\n")
				}
			}
		}

		{ // optionally run 'build-hook-post' if existing
			if isExecutable(buildHookPostPath) {
				run([]string{buildHookPostPath}, nil)
			}
		}
	}
}
