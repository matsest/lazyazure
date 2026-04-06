#!/bin/bash
# Test search functionality using tmux

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

SESSION="lazyazure-search-test-$RANDOM"
DIMENSIONS="-x 120 -y 40"
FAILED=0

# Cleanup function
cleanup() {
  tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

echo ""
echo "=== Testing Search Functionality ==="

# Create detached session
tmux new-session -d -s "$SESSION" $DIMENSIONS

# Start app in demo mode
echo "Starting lazyazure..."
tmux send-keys -t "$SESSION" "cd $(pwd) && LAZYAZURE_DEBUG=1 LAZYAZURE_DEMO=1 ./lazyazure" Enter

# Wait for app initialization
sleep 3

# Capture initial state
echo "Capturing initial state..."
tmux capture-pane -t "$SESSION" -p >/tmp/search-test-initial.txt

# Test 1: Verify app started
echo "Test 1: Verify app started..."
if grep -q "Subscriptions" /tmp/search-test-initial.txt; then
  echo -e "${GREEN}✓${NC} App started successfully"
else
  echo -e "${RED}✗${NC} FAIL: App did not start properly"
  cat /tmp/search-test-initial.txt
  FAILED=1
fi

# Navigate to resource groups panel
echo "Navigating to Resource Groups panel..."
tmux send-keys -t "$SESSION" Tab
sleep 0.5
tmux send-keys -t "$SESSION" Enter
sleep 1

# Test 2: Activate search mode
echo "Test 2: Activate search mode with /..."
tmux send-keys -t "$SESSION" "/"
sleep 0.5

tmux capture-pane -t "$SESSION" -p >/tmp/search-test-activated.txt

# Check that search prompt appears (look for "/" at bottom)
if grep -q "^/" /tmp/search-test-activated.txt || tail -1 /tmp/search-test-activated.txt | grep -q "/"; then
  echo -e "${GREEN}✓${NC} Search mode activated"
else
  echo -e "${RED}✗${NC} FAIL: Search mode not activated"
  tail -5 /tmp/search-test-activated.txt
  FAILED=1
fi

# Test 3: Type characters
echo "Test 3: Typing characters..."
tmux send-keys -t "$SESSION" "e"
sleep 0.3
tmux send-keys -t "$SESSION" "a"
sleep 0.3
tmux send-keys -t "$SESSION" "s"
sleep 0.3

tmux capture-pane -t "$SESSION" -p >/tmp/search-test-typed.txt

# Check that text appears
if tail -1 /tmp/search-test-typed.txt | grep -q "eas"; then
  echo -e "${GREEN}✓${NC} Characters typed successfully"
else
  echo -e "${RED}✗${NC} FAIL: Characters not appearing"
  tail -3 /tmp/search-test-typed.txt
  FAILED=1
fi

# Test 4: Backspace functionality
echo "Test 4: Testing backspace..."
tmux send-keys -t "$SESSION" Bspace
sleep 0.3

tmux capture-pane -t "$SESSION" -p >/tmp/search-test-backspace.txt

if tail -1 /tmp/search-test-backspace.txt | grep -q "ea"; then
  echo -e "${GREEN}✓${NC} Backspace working"
else
  echo -e "${RED}✗${NC} FAIL: Backspace not working"
  tail -3 /tmp/search-test-backspace.txt
  FAILED=1
fi

# Test 5: Cancel search with Escape
echo "Test 5: Cancel search with Escape..."
tmux send-keys -t "$SESSION" Escape
sleep 0.5

tmux capture-pane -t "$SESSION" -p >/tmp/search-test-cancelled.txt

# Check that filter status is cleared from status bar
if grep -q "Filter:" /tmp/search-test-cancelled.txt; then
  echo -e "${RED}✗${NC} FAIL: Filter still showing after cancel"
  tail -3 /tmp/search-test-cancelled.txt
  FAILED=1
else
  echo -e "${GREEN}✓${NC} Search cancelled successfully"
fi

# Test 6: Confirm search with Enter
echo "Test 6: Test confirm search..."
tmux send-keys -t "$SESSION" "/"
sleep 0.3
tmux send-keys -t "$SESSION" "p"
sleep 0.3
tmux send-keys -t "$SESSION" "r"
sleep 0.3
tmux send-keys -t "$SESSION" Enter
sleep 0.5

tmux capture-pane -t "$SESSION" -p >/tmp/search-test-confirmed.txt

# After confirming, should see filter in status bar
if grep -q "Filter:" /tmp/search-test-confirmed.txt || grep -q "/: Search" /tmp/search-test-confirmed.txt; then
  echo -e "${GREEN}✓${NC} Search confirmed"
else
  echo -e "${RED}✗${NC} FAIL: Search not confirmed properly"
  tail -3 /tmp/search-test-confirmed.txt
  FAILED=1
fi

# Check debug log for errors
echo "=== Checking debug log ==="
DEBUG_LOG="$HOME/.lazyazure/debug.log"
if [ -f "$DEBUG_LOG" ]; then
  # Check for hang/deadlock indicators
  if tail -20 "$DEBUG_LOG" | grep -q "callback completed"; then
    echo -e "${GREEN}✓${NC} No deadlock detected in debug log"
  else
    echo -e "${YELLOW}WARNING${NC}: Could not verify callback completion"
  fi

  # Check for errors
  if tail -50 "$DEBUG_LOG" | grep -i "error\|fatal\|panic" | grep -v "grep"; then
    echo -e "${RED}✗${NC} Errors found in debug log"
    FAILED=1
  else
    echo -e "${GREEN}✓${NC} No errors in debug log"
  fi
else
  echo -e "${YELLOW}WARNING${NC}: Debug log not found (LAZYAZURE_DEBUG may not be set)"
fi

# Summary
echo ""
echo "=== Test Summary ==="
if [ $FAILED -eq 0 ]; then
  echo -e "${GREEN}✓${NC} All search tests passed!"
  exit 0
else
  echo -e "${RED}✗${NC} Some search tests failed"
  exit 1
fi
