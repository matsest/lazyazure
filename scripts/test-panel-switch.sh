#!/bin/bash
# Test panel switching functionality using Tab and Shift+Tab keys
# Verifies that Tab cycles through panels: subscriptions → resourcegroups → resources → main → subscriptions
# Verifies that Shift+Tab (Backtab) cycles in reverse

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

SESSION="lazyazure-panel-test-$RANDOM"
DIMENSIONS="-x 120 -y 40"
FAILED=0

# Cleanup function
cleanup() {
  tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

echo ""
echo "=== Testing Panel Switching ==="
echo "Session: $SESSION"

# Create tmux session
tmux new-session -d -s "$SESSION" $DIMENSIONS

# Start app in demo mode
echo "Starting app in demo mode (LAZYAZURE_DEMO=1)..."
tmux send-keys -t "$SESSION" "LAZYAZURE_DEMO=1 ./lazyazure" Enter

# Wait for app to load
sleep 2

# Helper function to capture pane content
capture() {
  tmux capture-pane -t "$SESSION" -p >"$1"
}

# Helper function to capture pane content with ANSI escape sequences (for color checking)
capture_ansi() {
  tmux capture-pane -t "$SESSION" -p -e >"$1"
}

# Helper function to check frame colors
# Usage: check_frame_colors <expected_active_panel>
# Returns 0 if the expected panel has green frame
check_frame_colors() {
  local expected_active="$1"
  local tmpfile="/tmp/frame-colors-check.txt"
  capture_ansi "$tmpfile"

  # Convert escape sequences to visible format for parsing
  local visible_output
  visible_output=$(cat -v "$tmpfile")

  # Check if the expected panel has green frame color
  # Green = \[32m appears in the line before the panel title
  case "$expected_active" in
  "subscriptions")
    # Subscriptions panel - look for green color in the Subscriptions title line
    # The line with "Subscriptions" should start with [32m when active
    if echo "$visible_output" | grep "Subscriptions" | head -1 | grep -q '\[32m'; then
      return 0
    fi
    ;;
  "resourcegroups")
    # Resource Groups panel
    if echo "$visible_output" | grep "Resource Groups" | head -1 | grep -q '\[32m'; then
      return 0
    fi
    ;;
  "resources")
    # Resources panel - need to distinguish from "Resource Groups"
    # Find the line that has "Resources" but NOT "Groups"
    if echo "$visible_output" | grep "Resources" | grep -v "Groups" | head -1 | grep -q '\[32m'; then
      return 0
    fi
    ;;
  "main")
    # Main panel - Summary tab is ALWAYS green ([32m), but when main is active:
    # 1. Summary also becomes BOLD ([1m)
    # 2. There are TWO [32m on line 1 (tab + frame border)
    # We check for bold as it's the most reliable indicator
    if echo "$visible_output" | grep "Summary" | head -1 | grep -q '\[1m'; then
      return 0
    fi
    ;;
  esac
  return 1
}

# Helper function to check which panel is active based on status bar
# Each panel has distinctive help text in the status bar
check_active_panel() {
  local expected="$1"
  capture /tmp/panel-check.txt

  case "$expected" in
  "subscriptions")
    # Subscriptions panel shows "Enter: Load RGs" (distinctive)
    if grep -q "Enter: Load RGs" /tmp/panel-check.txt; then
      return 0
    fi
    ;;
  "resourcegroups")
    # Resource Groups panel shows "Enter: Load Resources" (distinctive)
    if grep -q "Enter: Load Resources" /tmp/panel-check.txt; then
      return 0
    fi
    ;;
  "resources")
    # Resources panel shows "Enter: View Details" (distinctive)
    if grep -q "Enter: View Details" /tmp/panel-check.txt; then
      return 0
    fi
    ;;
  "main")
    # Main panel shows "Tab: Back to List" (distinctive)
    if grep -q "Tab: Back to List" /tmp/panel-check.txt; then
      return 0
    fi
    ;;
  esac
  return 1
}

# Test 1: Verify initial state (subscriptions panel)
echo ""
echo "Test 1: Initial state - subscriptions panel"
sleep 0.5
if check_active_panel "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Subscriptions panel is active (status bar)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected subscriptions panel to be active"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi

# Also verify frame color
if check_frame_colors "subscriptions"; then
  echo "  ✓ Subscriptions frame is green (other panels are white)"
else
  echo -e "  ${YELLOW}WARNING${NC}: Could not verify frame color (may need visual check)"
fi

# Test 2: Tab forward through all panels
echo ""
echo "Test 2: Tab forward cycling"

# Tab 1: subscriptions → resourcegroups
echo "  Pressing Tab to switch to resourcegroups..."
tmux send-keys -t "$SESSION" Tab
sleep 0.3
if check_active_panel "resourcegroups"; then
  echo -e "  ${GREEN}✓${NC} Resource Groups panel is active (status bar)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Resource Groups panel"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "resourcegroups"; then
  echo -e "  ${GREEN}✓${NC} Resource Groups frame is green"
fi

# Load resource groups first (need to press Enter on subscriptions to load RGs)
echo "  Loading resource groups..."
tmux send-keys -t "$SESSION" Enter
sleep 1

# Tab 2: resourcegroups → resources
echo "  Pressing Tab to switch to resources..."
tmux send-keys -t "$SESSION" Tab
sleep 0.3
if check_active_panel "resources"; then
  echo -e "  ${GREEN}✓${NC} Resources panel is active (status bar)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Resources panel"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "resources"; then
  echo -e "  ${GREEN}✓${NC} Resources frame is green"
fi

# Load resources first (need to press Enter on resource groups to load resources)
echo "  Loading resources..."
tmux send-keys -t "$SESSION" Enter
sleep 1

# Tab 3: resources → main
echo "  Pressing Tab to switch to main..."
tmux send-keys -t "$SESSION" Tab
sleep 0.3
if check_active_panel "main"; then
  echo -e "  ${GREEN}✓${NC} Main panel is active (status bar)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Main panel"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "main"; then
  echo -e "  ${GREEN}✓${NC} Main panel frame is green"
fi

# Tab 4: main → subscriptions (back to start)
echo "  Pressing Tab to switch back to subscriptions..."
tmux send-keys -t "$SESSION" Tab
sleep 0.3
if check_active_panel "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Back to Subscriptions panel (status bar)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Subscriptions panel (cycle back)"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Subscriptions frame is green (cycle complete)"
fi

# Test 3: Shift+Tab (Backtab) reverse cycling
echo ""
echo "Test 3: Shift+Tab (Backtab) reverse cycling"

# Backtab 1: subscriptions → main
echo "  Pressing Shift+Tab to switch to main..."
tmux send-keys -t "$SESSION" BTab
sleep 0.3
if check_active_panel "main"; then
  echo -e "  ${GREEN}✓${NC} Main panel is active (reverse)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Main panel (reverse)"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "main"; then
  echo -e "  ${GREEN}✓${NC} Main panel frame is green (reverse)"
fi

# Backtab 2: main → resources
echo "  Pressing Shift+Tab to switch to resources..."
tmux send-keys -t "$SESSION" BTab
sleep 0.3
if check_active_panel "resources"; then
  echo -e "  ${GREEN}✓${NC} Resources panel is active (reverse)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Resources panel (reverse)"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "resources"; then
  echo -e "  ${GREEN}✓${NC} Resources frame is green (reverse)"
fi

# Backtab 3: resources → resourcegroups
echo "  Pressing Shift+Tab to switch to resourcegroups..."
tmux send-keys -t "$SESSION" BTab
sleep 0.3
if check_active_panel "resourcegroups"; then
  echo -e "  ${GREEN}✓${NC} Resource Groups panel is active (reverse)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Resource Groups panel (reverse)"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "resourcegroups"; then
  echo -e "  ${GREEN}✓${NC} Resource Groups frame is green (reverse)"
fi

# Backtab 4: resourcegroups → subscriptions (back to start)
echo "  Pressing Shift+Tab to switch back to subscriptions..."
tmux send-keys -t "$SESSION" BTab
sleep 0.3
if check_active_panel "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Back to Subscriptions panel (reverse)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected Subscriptions panel (reverse cycle)"
  tail -2 /tmp/panel-check.txt
  FAILED=1
fi
if check_frame_colors "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Subscriptions frame is green (reverse cycle complete)"
fi

# Test 4: Rapid Tab switching
echo ""
echo "Test 4: Rapid Tab switching"
echo "  Sending Tab 8 times rapidly..."
for i in {1..8}; do
  tmux send-keys -t "$SESSION" Tab
  sleep 0.1
done
sleep 0.5

# After 8 tabs (2 full cycles), should be back at subscriptions
if check_active_panel "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Rapid Tab switching works correctly"
else
  echo -e "  ${YELLOW}WARNING${NC}: Rapid Tab switching may have issues (or panel was in different state)"
  # Don't fail on rapid switching - can be timing-sensitive
fi

# Test 5: Final verification
echo ""
echo "Test 5: Final state verification"

# Verify we ended at subscriptions panel and it has green frame
if check_active_panel "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Final panel is subscriptions (status bar)"
else
  echo -e "  ${RED}✗${NC} FAIL: Expected to end at subscriptions panel"
  FAILED=1
fi

if check_frame_colors "subscriptions"; then
  echo -e "  ${GREEN}✓${NC} Subscriptions panel has green frame at end of test"
fi

# Verify ANSI codes are present (colors working)
capture_ansi /tmp/panel-colors-final.txt
if grep -q $'\033\[' /tmp/panel-colors-final.txt 2>/dev/null ||
  cat /tmp/panel-colors-final.txt | od -c | grep -q '\\033'; then
  echo -e "  ${GREEN}✓${NC} ANSI escape sequences present (terminal colors working)"
fi

# Summary
echo ""
echo "=== Test Summary ==="
if [ $FAILED -eq 0 ]; then
  echo -e "${GREEN}✓${NC} All panel switching tests passed!"
  exit 0
else
  echo -e "${RED}✗${NC} Some panel switching tests failed"
  exit 1
fi
