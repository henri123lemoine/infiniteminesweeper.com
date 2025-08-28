#!/bin/bash

# Integration Test Runner for Infinite Minesweeper
# Runs comprehensive integration tests for both backend and frontend

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TEST_TIMEOUT=${TEST_TIMEOUT:-60s}
VERBOSE=${VERBOSE:-false}
# Default to skipping performance tests in CI/most environments; opt-in locally.
SKIP_PERFORMANCE=${SKIP_PERFORMANCE:-true}
BACKEND_ONLY=${BACKEND_ONLY:-false}
FRONTEND_ONLY=${FRONTEND_ONLY:-false}

# Test categories
BASIC_TESTS="TestMultiClientBoardConsistency TestMoveValidationPipeline TestConcurrentReveals TestReconnectionStateSync"
PERFORMANCE_TESTS="TestLoadWithManyClients TestMemoryUsageUnderLoad TestChunkGenerationPerformance TestWebSocketThroughput TestConcurrentChunkAccess"

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_dependencies() {
    log_info "Checking dependencies..."
    
    if ! command -v go &> /dev/null; then
        log_error "Go is not installed or not in PATH"
        exit 1
    fi
    if ! command -v make &> /dev/null; then
        log_error "make is not installed or not in PATH"
        exit 1
    fi
    
    if ! command -v node &> /dev/null; then
        log_error "Node.js is not installed or not in PATH"
        exit 1
    fi
    # Require Node 18+ for global fetch used in tests
    local node_ver
    node_ver=$(node -v 2>/dev/null | sed 's/^v//')
    local node_major=${node_ver%%.*}
    if [[ -z "$node_major" ]] || (( node_major < 18 )); then
        log_error "Node.js v18+ required (found v${node_ver:-unknown})"
        exit 1
    fi
    
    if ! command -v npm &> /dev/null; then
        log_error "npm is not installed or not in PATH"
        exit 1
    fi
    if ! command -v npx &> /dev/null; then
        log_error "npx is not installed or not in PATH"
        exit 1
    fi
    if ! command -v curl &> /dev/null; then
        log_error "curl is required for server health checks"
        exit 1
    fi
    if ! command -v protoc &> /dev/null; then
        log_error "protoc (Protocol Buffers compiler) is required; install it before running tests"
        exit 1
    fi
    
    # Check if we're in the right directory
    if [[ ! -f "go.mod" ]] || [[ ! -d "backend" ]] || [[ ! -d "frontend" ]]; then
        log_error "Must be run from the project root directory"
        exit 1
    fi
    
    log_success "Dependencies check passed"
}

build_project() {
    log_info "Building project via Makefile (go-build)..."
    # This builds frontend assets into backend/dist and then the Go backend embedding them
    make go-build || {
        log_error "Failed to build project via make go-build"
        exit 1
    }
    log_success "Project built successfully (backend/dist/backend)"
}

run_backend_tests() {
    log_info "Running backend integration tests..."
    
    local test_args="-timeout ${TEST_TIMEOUT}"
    if [[ "$VERBOSE" == "true" ]]; then
        test_args="$test_args -v"
    fi
    
    local failed_tests=()
    
    # Run basic integration tests
    log_info "Running basic integration tests..."
    for test in $BASIC_TESTS; do
        log_info "  Running $test..."
        if go test $test_args ./backend -run "^${test}$"; then
            log_success "    ✓ $test"
        else
            log_error "    ✗ $test"
            failed_tests+=("$test")
        fi
    done
    
    # Run performance tests (unless skipped)
    if [[ "$SKIP_PERFORMANCE" != "true" ]]; then
        log_info "Running performance tests..."
        for test in $PERFORMANCE_TESTS; do
            log_info "  Running $test..."
            if go test $test_args ./backend -run "^${test}$"; then
                log_success "    ✓ $test"
            else
                log_warning "    ⚠ $test (performance test failed)"
                # Don't fail the entire suite on performance test failures
            fi
        done
    else
        log_warning "Skipping performance tests (SKIP_PERFORMANCE=true)"
    fi
    
    # Run benchmarks
    log_info "Running benchmarks..."
    go test -bench=. -benchmem ./backend -run "^$" || {
        log_warning "Some benchmarks failed"
    }
    
    if [[ ${#failed_tests[@]} -gt 0 ]]; then
        log_error "Backend tests failed: ${failed_tests[*]}"
        return 1
    else
        log_success "All backend integration tests passed"
        return 0
    fi
}

start_test_server() {
    log_info "Starting test server..."
    
    # Start server in background
    backend/dist/backend &
    local server_pid=$!
    echo $server_pid > /tmp/minesweeper-test.pid
    
    # Wait for server to be ready
    local max_attempts=30
    local attempt=0
    
    while [[ $attempt -lt $max_attempts ]]; do
        if curl -s http://localhost:8080/ &> /dev/null; then
            log_success "Test server started (PID: $server_pid)"
            return 0
        fi
        
        sleep 1
        ((attempt++))
    done
    
    log_error "Test server failed to start within ${max_attempts} seconds"
    kill $server_pid 2>/dev/null || true
    return 1
}

stop_test_server() {
    if [[ -f /tmp/minesweeper-test.pid ]]; then
        local server_pid=$(cat /tmp/minesweeper-test.pid)
        log_info "Stopping test server (PID: $server_pid)..."
        
        kill $server_pid 2>/dev/null || true
        rm -f /tmp/minesweeper-test.pid
        
        # Wait for server to stop
        local attempt=0
        while kill -0 $server_pid 2>/dev/null && [[ $attempt -lt 10 ]]; do
            sleep 1
            ((attempt++))
        done
        
        if kill -0 $server_pid 2>/dev/null; then
            log_warning "Force killing server..."
            kill -9 $server_pid 2>/dev/null || true
        fi
        
        log_success "Test server stopped"
    fi
}

run_frontend_tests() {
    log_info "Running frontend integration tests..."
    
    # Start test server if not already running
    if ! curl -s http://localhost:8080/ &> /dev/null; then
        start_test_server || {
            log_error "Failed to start test server for frontend tests"
            return 1
        }
        local server_started=true
    fi
    
    # Run frontend integration tests
    cd frontend
    if node tests/integration-tests.mjs; then
        log_success "Frontend integration tests passed"
        local frontend_result=0
    else
        log_error "Frontend integration tests failed"
        local frontend_result=1
    fi
    cd ..
    
    # Stop server if we started it
    if [[ "${server_started:-false}" == "true" ]]; then
        stop_test_server
    fi
    
    return $frontend_result
}

run_end_to_end_tests() {
    log_info "Running end-to-end integration tests..."
    
    # Start server
    start_test_server || {
        log_error "Failed to start server for e2e tests"
        return 1
    }
    
    # Run combined tests that exercise both backend and frontend
    # TODO: Deduplicate or differentiate from run_frontend_tests; currently invokes the same suite.
    log_info "Testing full client-server flow..."
    
    # This could include tests like:
    # - Multiple browser instances interacting
    # - Load testing with real WebSocket clients
    # - State synchronization across reconnects
    
    local e2e_result=0
    
    # Run frontend tests against live server
    cd frontend
    if node tests/integration-tests.mjs; then
        log_success "End-to-end tests passed"
    else
        log_error "End-to-end tests failed"
        e2e_result=1
    fi
    cd ..
    
    stop_test_server
    return $e2e_result
}

generate_test_report() {
    local start_time=$1
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    log_info "Generating test report..."
    
    cat > ../data/test-report.txt << EOF
Integration Test Report
=======================
Date: $(date)
Duration: ${duration}s
Environment: $(go version)
Node: $(node --version)

Test Configuration:
- Timeout: $TEST_TIMEOUT
- Skip Performance: $SKIP_PERFORMANCE
- Backend Only: $BACKEND_ONLY
- Frontend Only: $FRONTEND_ONLY
- Verbose: $VERBOSE

Tests Run:
- Backend Integration Tests: $(if [[ "$FRONTEND_ONLY" != "true" ]]; then echo "✓"; else echo "SKIPPED"; fi)
- Frontend Integration Tests: $(if [[ "$BACKEND_ONLY" != "true" ]]; then echo "✓"; else echo "SKIPPED"; fi)
- Performance Tests: $(if [[ "$SKIP_PERFORMANCE" != "true" ]]; then echo "✓"; else echo "SKIPPED"; fi)

For detailed results, see the test output above.
EOF
    
    log_success "Test report generated: test-report.txt"
}

cleanup() {
    log_info "Cleaning up..."
    stop_test_server
    rm -f /tmp/minesweeper-test
}

show_help() {
    cat << EOF
Integration Test Runner for Infinite Minesweeper

Usage: $0 [OPTIONS]

OPTIONS:
  -h, --help              Show this help message
  -v, --verbose           Enable verbose test output
  -t, --timeout TIMEOUT   Set test timeout (default: 60s)
  --skip-performance      Skip performance tests
  --backend-only          Run only backend tests
  --frontend-only         Run only frontend tests
  --no-cleanup            Don't clean up temporary files

ENVIRONMENT VARIABLES:
  TEST_TIMEOUT           Test timeout (default: 60s)
  VERBOSE               Enable verbose output (default: false)
  SKIP_PERFORMANCE      Skip performance tests (default: false)
  BACKEND_ONLY          Run backend tests only (default: false)
  FRONTEND_ONLY         Run frontend tests only (default: false)

EXAMPLES:
  $0                      # Run all tests
  $0 --backend-only       # Run only backend tests
  $0 --skip-performance   # Skip performance tests
  $0 -v -t 120s          # Verbose output with 2min timeout
EOF
}

main() {
    local start_time=$(date +%s)
    local overall_result=0
    
    # Parse command line arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                show_help
                exit 0
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -t|--timeout)
                TEST_TIMEOUT="$2"
                shift 2
                ;;
            --skip-performance)
                SKIP_PERFORMANCE=true
                shift
                ;;
            --backend-only)
                BACKEND_ONLY=true
                shift
                ;;
            --frontend-only)
                FRONTEND_ONLY=true
                shift
                ;;
            --no-cleanup)
                NO_CLEANUP=true
                shift
                ;;
            *)
                log_error "Unknown option: $1"
                show_help
                exit 1
                ;;
        esac
    done
    
    # Validate conflicting options
    if [[ "$BACKEND_ONLY" == "true" ]] && [[ "$FRONTEND_ONLY" == "true" ]]; then
        log_error "Cannot specify both --backend-only and --frontend-only"
        exit 1
    fi
    
    # Set up cleanup trap
    if [[ "${NO_CLEANUP:-false}" != "true" ]]; then
        trap cleanup EXIT
    fi
    
    log_info "Starting integration tests..."
    log_info "Configuration: timeout=$TEST_TIMEOUT, verbose=$VERBOSE, skip_performance=$SKIP_PERFORMANCE"
    
    # Run the test pipeline
    check_dependencies || exit 1
    build_project || exit 1
    
    # Run backend tests
    if [[ "$FRONTEND_ONLY" != "true" ]]; then
        if ! run_backend_tests; then
            overall_result=1
        fi
    fi
    
    # Run frontend tests
    if [[ "$BACKEND_ONLY" != "true" ]]; then
        if ! run_frontend_tests; then
            overall_result=1
        fi
        
        # Run end-to-end tests
        if ! run_end_to_end_tests; then
            overall_result=1
        fi
    fi
    
    # Generate report
    generate_test_report $start_time
    
    # Final result
    if [[ $overall_result -eq 0 ]]; then
        log_success "🎉 All integration tests passed!"
        exit 0
    else
        log_error "💥 Some integration tests failed!"
        exit 1
    fi
}

# Run main function with all arguments
main "$@"
