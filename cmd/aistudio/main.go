package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof" // Register pprof handlers
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio" // Adjust import path if necessary
	"github.com/tmc/aistudio/api"
)

// setupLogging directs log output to a file for easier debugging.
func setupLogging() *os.File {
	logFilePath := "aistudio-debug.log"
	// Use tea's helper to create the log file
	// This helper ensures the log file is created in a standard location if possible
	f, err := tea.LogToFile(logFilePath, "aistudio")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening log file '%s': %v\n", logFilePath, err)
		return nil
	}
	// fmt print a line clear ansi code:
	//fmt.Fprintf(os.Stderr, "Logging enabled: %s\n", logFilePath) // Inform user where logs are
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile) // Add timestamp and file info to logs
	log.SetOutput(f)                                     // Redirect standard logger
	return f
}

func main() {
	// --- Command Line Flags ---
	modelFlag := flag.String("model", aistudio.DefaultModel, "Gemini model ID to use.")
	audioFlag := flag.Bool("audio", true, "Enable audio output.")
	voiceFlag := flag.String("voice", aistudio.DefaultVoice, "Voice for audio output (e.g., Puck, Amber).")
	playerCmdFlag := flag.String("player", "", "Override command for audio playback (e.g., 'ffplay ...'). Auto-detected if empty.")
	apiKeyFlag := flag.String("api-key", "", "Gemini API Key (overrides GEMINI_API_KEY env var).")

	// New flags for history and tools
	historyFlag := flag.Bool("history", true, "Enable chat history.")
	historyDirFlag := flag.String("history-dir", "./history", "Directory for storing chat history.")
	toolsFlag := flag.Bool("tools", true, "Enable tool calling support.")
	toolsFileFlag := flag.String("tools-file", "", "JSON file containing tool definitions to load.")
	systemPromptFlag := flag.String("system-prompt", "", "System prompt to use for the conversation.")
	systemPromptFileFlag := flag.String("system-prompt-file", "", "Load system prompt from a file.")
	listModelsFlag := flag.Bool("list-models", false, "List available models and exit.")
	filterModelsFlag := flag.String("filter-models", "", "Filter models list (used with --list-models)")

	// Generation parameters
	temperatureFlag := flag.Float64("temperature", 0.7, "Temperature for text generation (0.0-1.0).")
	topPFlag := flag.Float64("top-p", 0.95, "Top-p value for text generation (0.0-1.0).")
	topKFlag := flag.Int("top-k", 40, "Top-k value for text generation.")
	maxOutputTokensFlag := flag.Int("max-output-tokens", 8192, "Maximum number of tokens to generate.")

	// Feature flags
	webSearchFlag := flag.Bool("web-search", false, "Enable web search capabilities.")
	codeExecutionFlag := flag.Bool("code-execution", false, "Enable code execution capabilities.")
	displayTokensFlag := flag.Bool("display-tokens", false, "Display token counts in the UI.")
	responseMimeTypeFlag := flag.String("response-mime-type", "", "Expected response MIME type (e.g., application/json).")
	responseSchemaFileFlag := flag.String("response-schema-file", "", "Path to JSON schema file defining response structure.")
	globalTimeoutFlag := flag.Duration("global-timeout", 0, "Global timeout for all API requests (e.g., 30s, 1m). Zero means no timeout.")
	toolApprovalFlag := flag.Bool("tool-approval", false, "Require user approval for tool calls.")
	stdinModeFlag := flag.Bool("stdin", false, "Read messages from stdin without running TUI. Useful for scripting.")

	// Profiling flags (all with pprof- prefix)
	cpuprofile := flag.String("pprof-cpu", "", "Write cpu profile to `file`")
	memprofile := flag.String("pprof-mem", "", "Write memory profile to `file`")
	traceFile := flag.String("pprof-trace", "", "Write execution trace to `file`")
	pprofServer := flag.String("pprof-server", "", "Enable pprof HTTP server on given address (e.g., 'localhost:6060')")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Interactive chat with Gemini Live Streaming API.\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  GEMINI_API_KEY: API Key (used if --api-key is not set).\n")
		fmt.Fprintf(os.Stderr, "\nTimeout Examples:\n")
		fmt.Fprintf(os.Stderr, "  30 second timeout: %s --global-timeout=30s\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  5 minute timeout: %s --global-timeout=5m\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nStdin Mode Examples:\n")
		fmt.Fprintf(os.Stderr, "  Interactive: cat | %s --stdin\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Piped: echo \"Hello\" | %s --stdin\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nProfiling Examples:\n")
		fmt.Fprintf(os.Stderr, "  CPU profile:    %s --pprof-cpu=cpu.prof\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Memory profile: %s --pprof-mem=mem.prof\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Execution trace: %s --pprof-trace=trace.out\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  HTTP server:    %s --pprof-server=localhost:6060\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "                  # Then visit http://localhost:6060/debug/pprof/\n")
	}
	flag.Parse()

	// --- Set up logging first ---
	logFile := setupLogging()
	if logFile != nil {
		defer logFile.Close()
		log.Println("--- Application Start ---")
		log.Printf("CLI Flags: model=%q audio=%t voice=%q player=%q api-key-set=%t list-models=%t",
			*modelFlag, *audioFlag, *voiceFlag, *playerCmdFlag, *apiKeyFlag != "", *listModelsFlag)
	} else {
		// Disable standard logger output if file logging failed, to avoid cluttering stderr
		log.SetOutput(io.Discard)
	}

	// Handle --list-models flag
	if *listModelsFlag {
		fmt.Println("Fetching available models...")
		apiKey := *apiKeyFlag
		if apiKey == "" {
			apiKey = os.Getenv("GEMINI_API_KEY")
			if apiKey == "" {
				fmt.Fprintln(os.Stderr, "Warning: No API key provided for listing models. Some models might not be visible.")
			}
		}

		// Initialize a client and options just for listing models
		client := &api.Client{
			APIKey: apiKey,
		}

		models, err := client.ListModels(*filterModelsFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Found %d models", len(models))
		if *filterModelsFlag != "" {
			fmt.Printf(" matching filter: %q", *filterModelsFlag)
		}
		fmt.Println()
		fmt.Println("==========")

		// Sort and display models
		sort.Strings(models)
		for _, model := range models {
			fmt.Println(model)
		}
		fmt.Println("==========")
		os.Exit(0)
	}

	// CPU profiling
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatalf("Could not create CPU profile: %v", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatalf("Could not start CPU profile: %v", err)
		}
		defer pprof.StopCPUProfile()
		log.Printf("CPU profiling enabled, writing to %s", *cpuprofile)
	}

	// Execution tracing
	if *traceFile != "" {
		f, err := os.Create(*traceFile)
		if err != nil {
			log.Fatalf("Could not create trace file: %v", err)
		}
		defer f.Close()
		if err := trace.Start(f); err != nil {
			log.Fatalf("Could not start trace: %v", err)
		}
		defer trace.Stop()
		log.Printf("Execution tracing enabled, writing to %s", *traceFile)
	}

	// HTTP server for pprof
	if *pprofServer != "" {
		// Start a server in a separate goroutine
		go func() {
			log.Printf("Starting pprof HTTP server on %s", *pprofServer)
			log.Printf("Visit http://%s/debug/pprof/ to access profiles", *pprofServer)

			// Start the server and log any errors
			if err := http.ListenAndServe(*pprofServer, nil); err != nil {
				log.Printf("Error starting pprof HTTP server: %v", err)
				fmt.Fprintf(os.Stderr, "Error starting pprof HTTP server: %v\n", err)
			}
		}()
	}

	// --- Configuration ---
	// Determine API Key: Flag > Env Var > ADC
	apiKey := *apiKeyFlag
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "INFO: No API key provided via --api-key flag or GEMINI_API_KEY env var. Attempting Application Default Credentials (ADC).")
			log.Println("API Key not provided via flag or env, attempting ADC.")
		} else {
			log.Println("Using API Key from GEMINI_API_KEY environment variable.")
		}
	} else {
		log.Println("Using API Key from --api-key flag.")
	}

	// Load system prompt from file if specified
	systemPrompt := *systemPromptFlag
	if *systemPromptFileFlag != "" {
		data, err := os.ReadFile(*systemPromptFileFlag)
		if err != nil {
			log.Printf("Warning: Failed to read system prompt file: %v", err)
			fmt.Fprintf(os.Stderr, "Warning: Failed to read system prompt file: %v\n", err)
			time.Sleep(2 * time.Second) // Pause for 2 seconds before continuing
		} else {
			systemPrompt = string(data)
			log.Printf("Loaded system prompt from file: %s", *systemPromptFileFlag)
		}
	}

	// Construct component options
	opts := []aistudio.Option{
		aistudio.WithAPIKey(apiKey), // Pass determined key (or "" for ADC)
		aistudio.WithModel(*modelFlag),
		aistudio.WithAudioOutput(*audioFlag, *voiceFlag),
		aistudio.WithHistory(*historyFlag, *historyDirFlag),
		aistudio.WithTools(*toolsFlag),
		aistudio.WithGlobalTimeout(*globalTimeoutFlag),
		aistudio.WithToolApproval(*toolApprovalFlag),
	}

	// Add system prompt if specified
	if systemPrompt != "" {
		opts = append(opts, aistudio.WithSystemPrompt(systemPrompt))
	}

	// Add tools file if specified
	if *toolsFileFlag != "" {
		opts = append(opts, aistudio.WithToolsFile(*toolsFileFlag))
	}

	if *playerCmdFlag != "" {
		opts = append(opts, aistudio.WithAudioPlayerCommand(*playerCmdFlag))
	}

	// Add generation parameters
	opts = append(opts, aistudio.WithTemperature(float32(*temperatureFlag)))
	opts = append(opts, aistudio.WithTopP(float32(*topPFlag)))
	opts = append(opts, aistudio.WithTopK(int32(*topKFlag)))
	opts = append(opts, aistudio.WithMaxOutputTokens(int32(*maxOutputTokensFlag)))

	// Add feature flags
	opts = append(opts, aistudio.WithWebSearch(*webSearchFlag))
	opts = append(opts, aistudio.WithCodeExecution(*codeExecutionFlag))
	opts = append(opts, aistudio.WithDisplayTokenCounts(*displayTokensFlag))

	// Add response configuration if specified
	if *responseMimeTypeFlag != "" {
		opts = append(opts, aistudio.WithResponseMimeType(*responseMimeTypeFlag))
	}

	if *responseSchemaFileFlag != "" {
		opts = append(opts, aistudio.WithResponseSchema(*responseSchemaFileFlag))
	}

	// --- Initialize Component ---
	component := aistudio.New(opts...)

	// Handle stdin mode if flag is set
	if *stdinModeFlag {
		// In stdin mode, audio is disabled
		*audioFlag = false

		// Setup minimal logging for stdin mode
		if logFile != nil {
			log.Println("Running in stdin mode")
		}

		// Process messages from stdin
		if err := component.ProcessStdinMode(nil); err != nil {
			log.Printf("Error in stdin mode: %v", err)
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Normal TUI mode
		model, err := component.InitModel()
		if err != nil {
			log.Printf("Failed to initialize model: %v", err)
			fmt.Fprintf(os.Stderr, "Error initializing model: %v\n", err)
			os.Exit(1)
		}

		// --- Run Bubble Tea Program ---
		// Use options that help with input focus and program behavior
		p := tea.NewProgram(
			model,
			//tea.WithAltScreen(),
			tea.WithMouseCellMotion(), // Better mouse support
		)

		log.Println("Starting Bubble Tea program...")
		if _, err := p.Run(); err != nil {
			log.Printf("Error running program: %v", err)
			fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
			os.Exit(1)
		}
	}

	// Write memory profile at exit if requested
	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Printf("Could not create memory profile: %v", err)
			fmt.Fprintf(os.Stderr, "Could not create memory profile: %v\n", err)
		} else {
			defer f.Close()
			runtime.GC() // Get up-to-date statistics
			if err := pprof.WriteHeapProfile(f); err != nil {
				log.Printf("Could not write memory profile: %v", err)
				fmt.Fprintf(os.Stderr, "Could not write memory profile: %v\n", err)
			} else {
				log.Printf("Memory profile written to %s", *memprofile)
				fmt.Fprintf(os.Stderr, "Memory profile written to %s\n", *memprofile)
			}
		}
	}

	log.Println("--- Application End ---")
}
