# AIStudio Release Checklist

This checklist should be completed before any release to ensure quality and stability.

## Pre-Release Testing

### Code Quality
- [x] All code compiles without errors (`go build ./...`)
- [x] All existing tests pass (`go test ./...`)
- [ ] Test coverage meets minimum threshold (target: >60%, current: 48.5%)
- [ ] No critical linter warnings (`go vet ./...` - currently has mutex copy issues)
- [ ] Code is properly formatted (`gofmt -l .` shows unformatted files)

### Test Coverage Status
Current coverage by package:
- `github.com/tmc/aistudio`: 15.7% ⚠️ (needs improvement)
- `github.com/tmc/aistudio/api`: 46.5% ⚠️
- `github.com/tmc/aistudio/audio`: 0.0% ❌ (critical - no tests)
- `github.com/tmc/aistudio/audioplayer`: 73.5% ✅
- `github.com/tmc/aistudio/integration`: 0.0% ❌ (critical - no tests)
- `github.com/tmc/aistudio/internal/helpers`: 84.0% ✅
- `github.com/tmc/aistudio/internal/httprr`: 96.0% ✅
- `github.com/tmc/aistudio/session`: 66.3% ✅
- `github.com/tmc/aistudio/settings`: 100.0% ✅

### Critical Issues to Fix
1. **Mutex Copy Issues** - Multiple packages copying sync.Mutex values
   - `session/storage_providers.go`: Session struct copies
   - `video/video_streaming.go`: PerformanceMonitor struct copies
   - Main package: ToolDefinition and ToolResponse copies
   - Tests: Various test files copying locked structs

2. **Missing Test Coverage**
   - Audio package: 0% coverage (18+ functions untested)
   - Integration package: 0% coverage (79+ functions untested)
   - Main package: Low coverage at 15.7%

3. **Code Formatting** - 20+ files need formatting

## Functional Testing

### Core Features
- [ ] Basic chat functionality works
- [ ] Model selection and switching works
- [ ] API key authentication works
- [ ] Vertex AI authentication works

### Streaming Features
- [ ] Bidirectional streaming works
- [ ] Audio input/output works
- [ ] Voice Activity Detection (VAD) works
- [ ] Video streaming works
- [ ] Screen capture works

### Advanced Features
- [ ] Tool calling works
- [ ] Code execution works
- [ ] Function calling works
- [ ] External API integration works
- [ ] MCP protocol integration works

### Platform Testing
- [ ] macOS build and run
- [ ] Linux build and run
- [ ] Windows build and run (if supported)

## Performance Testing
- [ ] Memory usage is reasonable
- [ ] No goroutine leaks
- [ ] Connection resilience (reconnection works)
- [ ] Proper cleanup on shutdown

## Documentation
- [ ] README is up to date
- [ ] CLAUDE.md instructions are current
- [ ] API documentation is complete
- [ ] Tool documentation is complete
- [ ] Examples work correctly

## Release Process
- [ ] Version number updated
- [ ] CHANGELOG updated
- [ ] All tests pass
- [ ] Binary builds successfully
- [ ] Release notes prepared
- [ ] Git tag created
- [ ] GitHub release created
- [ ] Binary artifacts uploaded

## Post-Release
- [ ] Smoke test released binaries
- [ ] Monitor for critical issues
- [ ] Update documentation if needed

## Recommended Actions Before Release

1. **Immediate Priority**
   - Fix mutex copy issues in all packages
   - Add basic tests for audio package
   - Add basic tests for integration package
   - Format all Go files

2. **High Priority**
   - Improve main package test coverage to at least 30%
   - Improve API package test coverage to at least 60%
   - Run and fix all linter warnings

3. **Medium Priority**
   - Add integration tests for streaming features
   - Add performance benchmarks
   - Document all public APIs

## Notes
- Current overall test coverage: 48.5%
- Recommended minimum for release: 60%
- Critical packages with 0% coverage must have at least basic tests