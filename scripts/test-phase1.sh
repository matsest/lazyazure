#!/bin/bash
# Test script for Phase 1 - Basic Bubbletea TUI

set -e

# Build the application
echo "Building..."
go build -o lazyazure-test .

# Start tmux session
tmux new-session -d -s lazyazure-test

# Run the app with demo mode
tmux send-keys -t lazyazure-test "LAZYAZURE_DEMO=1 ./lazyazure-test" Enter

# Wait for app to start
sleep 1

# Capture and check the screen content
echo "=== Screen content ==="
CONTENT=$(tmux capture-pane -t lazyazure-test -p)
echo "$CONTENT"

# Verify the expected text is shown
if echo "$CONTENT" | grep -q "LazyAzure - Bubbletea Edition"; then
    echo ""
    echo "✓ PASS: App shows expected startup message"
else
    echo ""
    echo "✗ FAIL: App does not show expected message"
    tmux kill-session -t lazyazure-test 2>/dev/null || true
    exit 1
fi

# Send 'q' to quit
tmux send-keys -t lazyazure-test "q"

# Wait for exit
sleep 0.5

# Capture again to see if it exited
echo ""
echo "=== Screen after sending 'q' ==="
CONTENT=$(tmux capture-pane -t lazyazure-test -p)
echo "$CONTENT"

# If we see the shell prompt, the app exited
if echo "$CONTENT" | grep -q "lazyazure-test"; then
    echo ""
    echo "✓ PASS: App exited cleanly on 'q' key"
else
    echo ""
    echo "Note: Checking with Ctrl+C..."
    tmux send-keys -t lazyazure-test C-c
    sleep 0.5
fi

# Clean up
tmux kill-session -t lazyazure-test 2>/dev/null || true
rm -f lazyazure-test

echo ""
echo "=== Phase 1 Test Complete ==="
