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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tmc/aistudio" // Adjust import path if necessary
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

	// Profiling flags
	cpuprofile := flag.String("cpuprofile", "", "Write cpu profile to `file`")
	memprofile := flag.String("memprofile", "", "Write memory profile to `file`")
	traceFile := flag.String("trace", "", "Write execution trace to `file`")
	pprofServer := flag.String("pprof-server", "", "Enable pprof HTTP server on given address (e.g., 'localhost:6060')")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Interactive chat with Gemini Live Streaming API.\n\nOptions:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nEnvironment Variables:\n")
		fmt.Fprintf(os.Stderr, "  GEMINI_API_KEY: API Key (used if --api-key is not set).\n")
		fmt.Fprintf(os.Stderr, "\nProfiling Examples:\n")
		fmt.Fprintf(os.Stderr, "  CPU profile:    %s --cpuprofile=cpu.prof\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Memory profile: %s --memprofile=mem.prof\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  Execution trace: %s --trace=trace.out\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  HTTP server:    %s --pprof-server=localhost:6060\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "                  # Then visit http://localhost:6060/debug/pprof/\n")
	}
	flag.Parse()

	// --- Set up logging first ---
	logFile := setupLogging()
	if logFile != nil {
		defer logFile.Close()
		log.Println("--- Application Start ---")
		log.Printf("CLI Flags: model=%q audio=%t voice=%q player=%q api-key-set=%t",
			*modelFlag, *audioFlag, *voiceFlag, *playerCmdFlag, *apiKeyFlag != "")
	} else {
		// Disable standard logger output if file logging failed, to avoid cluttering stderr
		log.SetOutput(io.Discard)
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
		log.Println("CPU profiling enabled, writing to %s", *cpuprofile)
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
		log.Println("Execution tracing enabled, writing to %s", *traceFile)
	}

	// HTTP server for pprof
	if *pprofServer != "" {
		// Start a server in a separate goroutine
		go func() {
			log.Println("Starting pprof HTTP server on %s", *pprofServer)
			log.Println("Visit http://%s/debug/pprof/ to access profiles", *pprofServer)

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

	// Construct component options
	opts := []aistudio.Option{
		aistudio.WithAPIKey(apiKey), // Pass determined key (or "" for ADC)
		aistudio.WithModel(*modelFlag),
		aistudio.WithAudioOutput(*audioFlag, *voiceFlag),
	}
	if *playerCmdFlag != "" {
		opts = append(opts, aistudio.WithAudioPlayerCommand(*playerCmdFlag))
	}

	// --- Initialize Component ---
	component := aistudio.New(opts...)
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
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(), // Better mouse support
	)

	log.Println("Starting Bubble Tea program...")
	if _, err := p.Run(); err != nil {
		log.Printf("Error running program: %v", err)
		fmt.Fprintf(os.Stderr, "Error running program: %v\n", err)
		os.Exit(1)
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
