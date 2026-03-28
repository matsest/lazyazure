# TUI Testing with tmux

This document describes how to programmatically test the LazyAzure TUI using tmux for reliable, reproducible testing.

## Overview

Tmux provides programmatic control for testing TUI applications in a real terminal environment without manual interaction.

## Prerequisites

### Required
- `tmux` (v3.0+)
- `go` (for building the application)

### Optional (for screenshots)
- `grim` - Wayland screenshot tool (for Wayland users)
- `import` (ImageMagick) - X11 screenshot tool (for X11 users)
- `scrot` - Alternative X11 screenshot tool

**Note:** Screenshot tools require an active display server and visible terminal window.

## Text-Based Testing (Universal)

Text-based testing works in any environment including headless/CI systems. It uses `tmux capture-pane` to dump the terminal contents for assertions.

### Basic Testing Pattern

```bash
#!/bin/bash
set -e

SESSION="lazyazure-test-$RANDOM"
DIMENSIONS="-x 120 -y 40"

# Cleanup function
cleanup() {
    tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

# Create detached session with fixed size
tmux new-session -d -s "$SESSION" $DIMENSIONS

# Build and start the app in demo mode
tmux send-keys -t "$SESSION" "cd $(pwd) && go build . && LAZYAZURE_DEMO=1 ./lazyazure" Enter

# Wait for app initialization
sleep 2

# Capture initial state
tmux capture-pane -t "$SESSION" -p > /tmp/initial-state.txt

# Navigate and interact
tmux send-keys -t "$SESSION" Down
sleep 0.5
tmux send-keys -t "$SESSION" Tab
sleep 0.5

# Capture after navigation
tmux capture-pane -t "$SESSION" -p > /tmp/after-navigation.txt

# Verify expected content
if grep -q "Subscriptions" /tmp/after-navigation.txt; then
    echo "✓ Subscriptions panel visible"
else
    echo "✗ FAIL: Subscriptions panel not visible"
    exit 1
fi

if grep -q "Resource Groups" /tmp/after-navigation.txt; then
    echo "✓ Resource Groups panel visible"
else
    echo "✗ FAIL: Resource Groups panel not visible"
    exit 1
fi

# Test colors (check for ANSI escape sequences)
tmux capture-pane -t "$SESSION" -e -p > /tmp/with-colors.txt
if grep -q $'\e\[38;5;114m' /tmp/with-colors.txt; then
    echo "✓ Colors rendering correctly"
else
    echo "✗ Colors may not be rendering (ANSI codes not found)"
fi

echo "All tests passed!"
```

### Capture Options Reference

| Flag | Description |
|------|-------------|
| `-p` | Print to stdout (instead of paste buffer) |
| `-e` | Include escape sequences (preserves colors) |
| `-C` | Escape non-printable characters as octal |
| `-N` | Preserve trailing spaces |
| `-J` | Join wrapped lines and preserve trailing spaces |
| `-S -` | Capture from start of history |
| `-E -` | Capture to end of visible content |

### Common Verification Patterns

**Check for panel titles:**
```bash
grep -E "Subscriptions|Resource Groups|Resources" /tmp/output.txt
```

**Verify borders render correctly:**
```bash
grep -q "┌.*┐" /tmp/output.txt && echo "Borders OK" || echo "Border characters not found"
```

**Check active panel highlighting:**
```bash
# Look for green color (ANSI 38;5;114)
grep -c $'\e\[38;5;114m' /tmp/with-colors.txt
```

**Count visible items:**
```bash
grep -c "^  " /tmp/output.txt  # Items indented with 2 spaces
```

## Screenshot Testing (Requires Display)

Screenshot testing captures the actual visual appearance of the TUI. This requires:
- An active display server (Wayland or X11)
- The terminal window to be visible/focused
- A screenshot tool installed

### Wayland (using grim)

```bash
#!/bin/bash

SESSION="lazyazure-test-$RANDOM"
tmux new-session -d -s "$SESSION" -x 120 -y 40

# Start app
tmux send-keys -t "$SESSION" "LAZYAZURE_DEMO=1 ./lazyazure" Enter
sleep 2

# Capture full screen
grim /tmp/tmux-screenshot-initial.png

# Navigate
tmux send-keys -t "$SESSION" Down Down Tab
sleep 0.5
grim /tmp/tmux-screenshot-navigated.png

# Capture specific region (requires knowing terminal position)
# grim -g "100,100 800x600" /tmp/cropped-screenshot.png

tmux kill-session -t "$SESSION"
```

**Grim options:**
- `grim output.png` - Capture entire screen
- `grim -g "X,Y WxH" output.png` - Capture specific geometry
- `grim -t jpeg -q 90 output.jpg` - JPEG output with quality
- `grim -c output.png` - Include cursor in screenshot

### X11 (using ImageMagick import)

```bash
#!/bin/bash

SESSION="lazyazure-test-$RANDOM"
tmux new-session -d -s "$SESSION" -x 120 -y 40

tmux send-keys -t "$SESSION" "LAZYAZURE_DEMO=1 ./lazyazure" Enter
sleep 2

# Capture entire screen
import -window root /tmp/tmux-screenshot.png

# Capture specific window (requires window ID)
# import -window $(xdotool getactivewindow) /tmp/tmux-screenshot.png

tmux kill-session -t "$SESSION"
```

**Note:** For X11, you may need `xdotool` to get window IDs programmatically.

### Screenshot Testing Limitations

1. **Requires display:** Cannot run in pure headless environments
2. **Window focus:** Terminal must be visible, not minimized
3. **Screen clutter:** Captures entire screen, not just terminal
4. **Timing:** Need adequate sleep time for rendering

**Workaround for CI/CD:** Use text-based testing for automated checks, screenshots for local verification.

## Subagent Testing Pattern

When making UI changes, delegate testing to a subagent that executes tmux scripts:

### Subagent Responsibilities

1. **Environment Setup**
   - Create tmux session with specified dimensions
   - Build and start application

2. **Test Execution**
   - Send keystrokes to simulate user interactions
   - Capture pane contents at each step
   - Take screenshots if display available

3. **Verification**
   - Assert on text content
   - Check for visual elements (borders, colors)
   - Report failures with context

4. **Cleanup**
   - Kill tmux session
   - Remove temporary files

### Example Subagent Script

```bash
#!/bin/bash
# test-ui.sh - Run by subagent

SESSION="lazyazure-test"
FAILED=0

# Test 1: Initial render
test_initial_render() {
    tmux new-session -d -s "$SESSION" -x 80 -y 24
    tmux send-keys -t "$SESSION" "LAZYAZURE_DEMO=1 ./lazyazure" Enter
    sleep 2
    
    tmux capture-pane -t "$SESSION" -p > /tmp/test1.txt
    
    if ! grep -q "Auth" /tmp/test1.txt; then
        echo "FAIL: Auth panel not visible in initial render"
        FAILED=1
    fi
    
    tmux kill-session -t "$SESSION"
}

# Test 2: Navigation
test_navigation() {
    tmux new-session -d -s "$SESSION" -x 120 -y 40
    tmux send-keys -t "$SESSION" "LAZYAZURE_DEMO=1 ./lazyazure" Enter
    sleep 2
    
    # Navigate to subscriptions
    tmux send-keys -t "$SESSION" Tab
    sleep 0.5
    tmux send-keys -t "$SESSION" Down
    sleep 0.5
    
    tmux capture-pane -t "$SESSION" -p > /tmp/test2.txt
    
    # Check that something is selected (would need to verify visually)
    echo "Navigation test completed - check /tmp/test2.txt"
    
    tmux kill-session -t "$SESSION"
}

# Run tests
test_initial_render
test_navigation

if [ $FAILED -eq 0 ]; then
    echo "All UI tests passed"
    exit 0
else
    echo "Some UI tests failed"
    exit 1
fi
```

## Common Test Scenarios

### Scenario 1: Panel Layout Verification

Verify that all four panels render correctly at minimum terminal size:

```bash
tmux new-session -d -s test -x 80 -y 24
tmux send-keys -t test "LAZYAZURE_DEMO=1 ./lazyazure" Enter
sleep 2
tmux capture-pane -t test -p | grep -E "(Auth|Subscriptions|Resource Groups|Resources)"
```

### Scenario 2: Color Rendering

Ensure ANSI colors are being applied:

```bash
tmux capture-pane -t test -e -p | cat -v | grep -c "\[38;5;"
```

### Scenario 3: Border Characters

Verify Unicode box-drawing characters display correctly:

```bash
tmux capture-pane -t test -p | grep -E "[┌┐└┘│─]" | wc -l
```

### Scenario 4: Navigation Response

Test that arrow keys change the selected item:

```bash
# Capture before
tmux capture-pane -t test -p > /tmp/before.txt

# Navigate down 3 times
for i in 1 2 3; do
    tmux send-keys -t test Down
    sleep 0.3
done

# Capture after
tmux capture-pane -t test -p > /tmp/after.txt

# Compare (would need visual inspection or cursor position check)
diff /tmp/before.txt /tmp/after.txt && echo "No change detected" || echo "Content changed"
```

## Tips for Reliable Testing

1. **Use consistent dimensions:** Always specify `-x` and `-y` when creating sessions
2. **Adequate sleep:** Allow time for rendering between actions (0.3-0.5s minimum)
3. **Demo mode:** Use `LAZYAZURE_DEMO=1` to avoid Azure credential requirements
4. **Cleanup:** Always use `trap` to ensure tmux sessions are killed
5. **Random session names:** Use `$RANDOM` to avoid conflicts when running multiple tests
6. **Capture with colors:** Use `-e` flag when checking color rendering

## Troubleshooting

**Issue:** `tmux capture-pane` shows empty output
- **Solution:** Increase sleep time after starting app

**Issue:** ANSI codes not appearing with `-e` flag
- **Solution:** Verify the app is actually outputting colors

**Issue:** grim screenshots are blank
- **Solution:** Terminal window must be visible, not behind other windows

**Issue:** Tests pass locally but fail in CI
- **Solution:** CI is likely headless - use text-based testing only

## Integration with Go Tests

While this document focuses on bash/command-line testing, you can integrate tmux tests into Go:

```go
func TestTUILayout(t *testing.T) {
    cmd := exec.Command("bash", "scripts/test-ui.sh")
    output, err := cmd.CombinedOutput()
    if err != nil {
        t.Fatalf("UI test failed: %v\nOutput: %s", err, output)
    }
}
```

This allows running TUI tests alongside unit tests with `go test ./...`
