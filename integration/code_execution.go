package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// CodeExecutionManager manages live Python code execution
type CodeExecutionManager struct {
	// Configuration
	config              CodeExecutionConfig
	pythonPath          string
	workingDir          string
	
	// Execution state
	activeExecutions    map[string]*ExecutionContext
	executionsMutex     sync.RWMutex
	totalExecutions     int64
	
	// Environment
	virtualEnv          string
	installedPackages   map[string]string
	
	// Channels
	executionChan       chan ExecutionRequest
	resultChan          chan ExecutionResult
	
	// Lifecycle
	ctx                 context.Context
	cancel              context.CancelFunc
	
	// UI updates
	uiUpdateChan        chan tea.Msg
}

// CodeExecutionConfig holds configuration for code execution
type CodeExecutionConfig struct {
	Enabled             bool
	MaxConcurrentExecs  int
	ExecutionTimeout    time.Duration
	MaxMemoryMB         int
	MaxCPUPercent       int
	EnableNetworking    bool
	AllowedPackages     []string
	RestrictedPackages  []string
	WorkingDir          string
	EnableDataViz       bool
	EnableFileIO        bool
	SecurityLevel       string // "strict", "moderate", "permissive"
}

// ExecutionRequest represents a code execution request
type ExecutionRequest struct {
	ID          string
	Code        string
	Language    string
	Context     map[string]interface{}
	StreamID    string
	UserID      string
	Timestamp   time.Time
	Interactive bool
}

// ExecutionResult represents the result of code execution
type ExecutionResult struct {
	ID           string
	Success      bool
	Output       string
	Error        string
	Duration     time.Duration
	MemoryUsage  int64
	Plots        []PlotData
	DataFrames   []DataFrameInfo
	Files        []FileInfo
	Timestamp    time.Time
}

// ExecutionContext tracks an ongoing execution
type ExecutionContext struct {
	ID        string
	Process   *exec.Cmd
	StartTime time.Time
	Cancel    context.CancelFunc
}

// PlotData represents a generated plot
type PlotData struct {
	ID          string
	Title       string
	Format      string // "png", "svg", "html"
	Data        []byte
	Width       int
	Height      int
	Timestamp   time.Time
}

// DataFrameInfo represents information about a DataFrame
type DataFrameInfo struct {
	Name        string
	Shape       [2]int // [rows, cols]
	Columns     []string
	Types       map[string]string
	Sample      []map[string]interface{}
	Summary     map[string]interface{}
}

// FileInfo represents information about created files
type FileInfo struct {
	Name        string
	Path        string
	Size        int64
	Type        string
	Timestamp   time.Time
}

// DefaultCodeExecutionConfig returns default configuration
func DefaultCodeExecutionConfig() CodeExecutionConfig {
	return CodeExecutionConfig{
		Enabled:             true,
		MaxConcurrentExecs:  3,
		ExecutionTimeout:    30 * time.Second,
		MaxMemoryMB:         512,
		MaxCPUPercent:       80,
		EnableNetworking:    false,
		AllowedPackages:     []string{"pandas", "numpy", "matplotlib", "seaborn", "plotly", "scipy", "sklearn"},
		RestrictedPackages:  []string{"subprocess", "os", "sys", "requests", "urllib"},
		WorkingDir:          "/tmp/aistudio_code",
		EnableDataViz:       true,
		EnableFileIO:        true,
		SecurityLevel:       "moderate",
	}
}

// NewCodeExecutionManager creates a new code execution manager
func NewCodeExecutionManager(config CodeExecutionConfig, uiUpdateChan chan tea.Msg) *CodeExecutionManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	manager := &CodeExecutionManager{
		config:            config,
		activeExecutions:  make(map[string]*ExecutionContext),
		installedPackages: make(map[string]string),
		executionChan:     make(chan ExecutionRequest, 100),
		resultChan:        make(chan ExecutionResult, 100),
		ctx:               ctx,
		cancel:            cancel,
		uiUpdateChan:      uiUpdateChan,
	}
	
	// Initialize environment
	manager.initializeEnvironment()
	
	// Start processing loops
	go manager.processExecutions()
	go manager.processResults()
	
	return manager
}

// initializeEnvironment sets up the Python execution environment
func (cem *CodeExecutionManager) initializeEnvironment() {
	// Find Python executable
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		pythonPath, err = exec.LookPath("python")
		if err != nil {
			log.Printf("[CODE_EXECUTION] Python not found: %v", err)
			return
		}
	}
	cem.pythonPath = pythonPath
	
	// Create working directory
	if cem.config.WorkingDir == "" {
		cem.config.WorkingDir = "/tmp/aistudio_code"
	}
	
	if err := os.MkdirAll(cem.config.WorkingDir, 0755); err != nil {
		log.Printf("[CODE_EXECUTION] Failed to create working directory: %v", err)
		return
	}
	cem.workingDir = cem.config.WorkingDir
	
	// Check installed packages
	cem.checkInstalledPackages()
	
	log.Printf("[CODE_EXECUTION] Environment initialized: Python at %s, Working dir: %s", 
		cem.pythonPath, cem.workingDir)
}

// checkInstalledPackages checks which packages are available
func (cem *CodeExecutionManager) checkInstalledPackages() {
	for _, pkg := range cem.config.AllowedPackages {
		cmd := exec.Command(cem.pythonPath, "-c", fmt.Sprintf("import %s; print(%s.__version__)", pkg, pkg))
		output, err := cmd.Output()
		if err == nil {
			version := strings.TrimSpace(string(output))
			cem.installedPackages[pkg] = version
			log.Printf("[CODE_EXECUTION] Package %s version %s available", pkg, version)
		}
	}
}

// ExecuteCode executes Python code
func (cem *CodeExecutionManager) ExecuteCode(request ExecutionRequest) error {
	if !cem.config.Enabled {
		return fmt.Errorf("code execution is disabled")
	}
	
	// Check concurrent executions limit
	cem.executionsMutex.RLock()
	activeCount := len(cem.activeExecutions)
	cem.executionsMutex.RUnlock()
	
	if activeCount >= cem.config.MaxConcurrentExecs {
		return fmt.Errorf("maximum concurrent executions reached")
	}
	
	// Generate ID if not provided
	if request.ID == "" {
		request.ID = fmt.Sprintf("exec_%d_%d", time.Now().UnixNano(), cem.totalExecutions)
	}
	
	select {
	case cem.executionChan <- request:
		cem.totalExecutions++
		return nil
	case <-cem.ctx.Done():
		return fmt.Errorf("code execution manager stopped")
	}
}

// processExecutions processes code execution requests
func (cem *CodeExecutionManager) processExecutions() {
	for {
		select {
		case <-cem.ctx.Done():
			return
		case request := <-cem.executionChan:
			go cem.executeCode(request)
		}
	}
}

// executeCode executes a single code request
func (cem *CodeExecutionManager) executeCode(request ExecutionRequest) {
	startTime := time.Now()
	
	// Create execution context
	execCtx, cancel := context.WithTimeout(cem.ctx, cem.config.ExecutionTimeout)
	defer cancel()
	
	// Track execution
	execContext := &ExecutionContext{
		ID:        request.ID,
		StartTime: startTime,
		Cancel:    cancel,
	}
	
	cem.executionsMutex.Lock()
	cem.activeExecutions[request.ID] = execContext
	cem.executionsMutex.Unlock()
	
	defer func() {
		cem.executionsMutex.Lock()
		delete(cem.activeExecutions, request.ID)
		cem.executionsMutex.Unlock()
	}()
	
	// Prepare code for execution
	code := cem.prepareCode(request.Code)
	
	// Create temporary file
	tempFile, err := cem.createTempFile(request.ID, code)
	if err != nil {
		cem.sendResult(ExecutionResult{
			ID:        request.ID,
			Success:   false,
			Error:     fmt.Sprintf("Failed to create temp file: %v", err),
			Duration:  time.Since(startTime),
			Timestamp: time.Now(),
		})
		return
	}
	defer os.Remove(tempFile)
	
	// Execute code
	cmd := exec.CommandContext(execCtx, cem.pythonPath, tempFile)
	cmd.Dir = cem.workingDir
	
	// Set up environment
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "PYTHONPATH="+cem.workingDir)
	if !cem.config.EnableNetworking {
		cmd.Env = append(cmd.Env, "PYTHONNOUSERSITE=1")
	}
	
	// Capture output
	var output, errorOutput strings.Builder
	cmd.Stdout = &output
	cmd.Stderr = &errorOutput
	
	// Track process
	execContext.Process = cmd
	
	// Execute
	err = cmd.Run()
	
	// Process results
	result := ExecutionResult{
		ID:        request.ID,
		Success:   err == nil,
		Output:    output.String(),
		Error:     errorOutput.String(),
		Duration:  time.Since(startTime),
		Timestamp: time.Now(),
	}
	
	// Extract plots and data frames
	result.Plots = cem.extractPlots(request.ID)
	result.DataFrames = cem.extractDataFrames(request.ID)
	result.Files = cem.extractFiles(request.ID)
	
	cem.sendResult(result)
}

// prepareCode prepares code for safe execution
func (cem *CodeExecutionManager) prepareCode(code string) string {
	// Add security wrapper
	wrapper := `
import sys
import os
import matplotlib
matplotlib.use('Agg')  # Use non-interactive backend
import matplotlib.pyplot as plt
import warnings
warnings.filterwarnings('ignore')

# Set up plot saving
plot_counter = 0
def save_plot(title=""):
    global plot_counter
    plot_counter += 1
    filename = f"plot_{plot_counter}.png"
    plt.savefig(filename, dpi=100, bbox_inches='tight')
    plt.close()
    print(f"PLOT_SAVED:{filename}:{title}")

# Override show() to save instead
original_show = plt.show
plt.show = lambda: save_plot()

# Data analysis helpers
def describe_dataframe(df, name="dataframe"):
    print(f"DATAFRAME_INFO:{name}:{df.shape[0]}:{df.shape[1]}")
    print(f"DATAFRAME_COLUMNS:{name}:{','.join(df.columns)}")
    print(f"DATAFRAME_TYPES:{name}:{df.dtypes.to_dict()}")
    if len(df) > 0:
        print(f"DATAFRAME_SAMPLE:{name}:{df.head(3).to_json()}")

# Execute user code
try:
%s
except Exception as e:
    print(f"ERROR: {e}")
    import traceback
    traceback.print_exc()
`
	
	return fmt.Sprintf(wrapper, code)
}

// createTempFile creates a temporary Python file
func (cem *CodeExecutionManager) createTempFile(id, code string) (string, error) {
	filename := filepath.Join(cem.workingDir, fmt.Sprintf("exec_%s.py", id))
	
	file, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	_, err = file.WriteString(code)
	if err != nil {
		return "", err
	}
	
	return filename, nil
}

// extractPlots extracts generated plots
func (cem *CodeExecutionManager) extractPlots(id string) []PlotData {
	var plots []PlotData
	
	// Look for plot files
	files, err := filepath.Glob(filepath.Join(cem.workingDir, "plot_*.png"))
	if err != nil {
		return plots
	}
	
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		
		plot := PlotData{
			ID:        fmt.Sprintf("%s_%s", id, filepath.Base(file)),
			Title:     filepath.Base(file),
			Format:    "png",
			Data:      data,
			Timestamp: time.Now(),
		}
		
		plots = append(plots, plot)
		os.Remove(file) // Clean up
	}
	
	return plots
}

// extractDataFrames extracts DataFrame information from output
func (cem *CodeExecutionManager) extractDataFrames(id string) []DataFrameInfo {
	var dataframes []DataFrameInfo
	// This would parse the output for DATAFRAME_INFO markers
	// Implementation would depend on the actual output format
	return dataframes
}

// extractFiles extracts created files
func (cem *CodeExecutionManager) extractFiles(id string) []FileInfo {
	var files []FileInfo
	
	// Look for created files (excluding temp files)
	filepath.Walk(cem.workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		
		// Skip temp files
		if strings.Contains(path, "exec_") || strings.Contains(path, "plot_") {
			return nil
		}
		
		fileInfo := FileInfo{
			Name:      info.Name(),
			Path:      path,
			Size:      info.Size(),
			Type:      filepath.Ext(path),
			Timestamp: info.ModTime(),
		}
		
		files = append(files, fileInfo)
		return nil
	})
	
	return files
}

// processResults processes execution results
func (cem *CodeExecutionManager) processResults() {
	for {
		select {
		case <-cem.ctx.Done():
			return
		case result := <-cem.resultChan:
			// Log result
			if result.Success {
				log.Printf("[CODE_EXECUTION] Execution %s completed in %v", result.ID, result.Duration)
			} else {
				log.Printf("[CODE_EXECUTION] Execution %s failed: %s", result.ID, result.Error)
			}
			
			// Send UI update
			if cem.uiUpdateChan != nil {
				cem.uiUpdateChan <- CodeExecutionResultMsg{Result: result}
			}
		}
	}
}

// sendResult sends an execution result
func (cem *CodeExecutionManager) sendResult(result ExecutionResult) {
	select {
	case cem.resultChan <- result:
		// Success
	default:
		log.Printf("[CODE_EXECUTION] Result channel full, dropping result for %s", result.ID)
	}
}

// StopExecution stops a running execution
func (cem *CodeExecutionManager) StopExecution(id string) error {
	cem.executionsMutex.RLock()
	execContext, exists := cem.activeExecutions[id]
	cem.executionsMutex.RUnlock()
	
	if !exists {
		return fmt.Errorf("execution %s not found", id)
	}
	
	// Cancel the execution
	execContext.Cancel()
	
	// Kill the process if it's running
	if execContext.Process != nil {
		if err := execContext.Process.Process.Kill(); err != nil {
			log.Printf("[CODE_EXECUTION] Failed to kill process for %s: %v", id, err)
		}
	}
	
	return nil
}

// GetActiveExecutions returns currently active executions
func (cem *CodeExecutionManager) GetActiveExecutions() map[string]*ExecutionContext {
	cem.executionsMutex.RLock()
	defer cem.executionsMutex.RUnlock()
	
	active := make(map[string]*ExecutionContext)
	for k, v := range cem.activeExecutions {
		active[k] = v
	}
	
	return active
}

// GetInstalledPackages returns installed packages
func (cem *CodeExecutionManager) GetInstalledPackages() map[string]string {
	return cem.installedPackages
}

// GetStatistics returns execution statistics
func (cem *CodeExecutionManager) GetStatistics() map[string]interface{} {
	cem.executionsMutex.RLock()
	defer cem.executionsMutex.RUnlock()
	
	return map[string]interface{}{
		"active_executions": len(cem.activeExecutions),
		"total_executions":  cem.totalExecutions,
		"python_path":       cem.pythonPath,
		"working_dir":       cem.workingDir,
		"installed_packages": len(cem.installedPackages),
	}
}

// Stop gracefully stops the code execution manager
func (cem *CodeExecutionManager) Stop() {
	// Stop all active executions
	cem.executionsMutex.RLock()
	for id := range cem.activeExecutions {
		cem.StopExecution(id)
	}
	cem.executionsMutex.RUnlock()
	
	cem.cancel()
	close(cem.executionChan)
	close(cem.resultChan)
	
	log.Printf("[CODE_EXECUTION] Manager stopped. Total executions: %d", cem.totalExecutions)
}

// UI Messages
type CodeExecutionResultMsg struct {
	Result ExecutionResult
}

type CodeExecutionStartedMsg struct {
	Request ExecutionRequest
}

type CodeExecutionStoppedMsg struct {
	ID string
}