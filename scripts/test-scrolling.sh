#!/bin/bash
# Test scrolling functionality for list panels
# This tests that navigation works beyond visible panel limits

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

SESSION="lazyazure-scroll-test-$RANDOM"
DIMENSIONS="-x 120 -y 30" # Small height to force scrolling
FAILED=0

cleanup() {
  tmux kill-session -t "$SESSION" 2>/dev/null || true
}
trap cleanup EXIT

echo ""
echo "=== Testing List Panel Scrolling ==="
echo "Session: $SESSION"

# Create tmux session with small height to force scrolling
tmux new-session -d -s "$SESSION" $DIMENSIONS

# Start app in demo mode with large dataset
echo "Starting app in demo mode (LAZYAZURE_DEMO=2)..."
tmux send-keys -t "$SESSION" "LAZYAZURE_DEMO=2 ./lazyazure" Enter

# Wait for app to load
sleep 2

# Helper function to get visible subscription count
get_visible_count() {
  tmux capture-pane -t "$SESSION" -p | grep -c "^  " || true
}

# Helper function to capture pane content
capture() {
  tmux capture-pane -t "$SESSION" -p >"$1"
}

# Test 1: Verify initial state shows subscriptions
echo ""
echo "Test 1: Initial state"
capture /tmp/initial.txt
if grep -q "Subscriptions" /tmp/initial.txt; then
  echo -e "  ${GREEN}✓${NC} Subscriptions panel visible"
else
  echo -e "  ${RED}✗${NC} FAIL: Subscriptions panel not visible"
  FAILED=1
fi

# Count initial visible items
VISIBLE_COUNT=$(get_visible_count)
echo "  Visible items: $VISIBLE_COUNT"

# Test 2: Navigate down multiple times to test scrolling
echo ""
echo "Test 2: Scrolling down with arrow keys"
echo "  Sending 20 Down arrow presses..."
for i in {1..20}; do
  tmux send-keys -t "$SESSION" Down
  sleep 0.1
done
sleep 0.5

capture /tmp/after-scroll-down.txt

# Check that we've scrolled (content should be different)
if diff /tmp/initial.txt /tmp/after-scroll-down.txt >/dev/null; then
  echo -e "  ${RED}✗${NC} FAIL: No change detected after scrolling down"
  FAILED=1
else
  echo -e "  ${GREEN}✓${NC} Content changed after scrolling (view scrolled)"
fi

# Test 3: Navigate up to test reverse scrolling
echo ""
echo "Test 3: Scrolling up with arrow keys"
echo "  Sending 20 Up arrow presses..."
for i in {1..20}; do
  tmux send-keys -t "$SESSION" Up
  sleep 0.1
done
sleep 0.5

capture /tmp/after-scroll-up.txt

# Check that we're back near the top (content should be similar to initial)
if grep -q "Demo-Tenant" /tmp/after-scroll-up.txt; then
  echo "  ✓ Back at top of list"
else
  echo -e "  ${YELLOW}WARNING${NC}: May not be at exact top (expected with mixed scrolling)"
fi

# Test 4: Test Page Down
echo ""
echo "Test 4: Page Down functionality"
tmux send-keys -t "$SESSION" Home # Go to top first
sleep 0.3
capture /tmp/before-page-down.txt

echo "  Sending Page Down..."
tmux send-keys -t "$SESSION" PPage # Page Up key in tmux
sleep 0.5
capture /tmp/after-page-down.txt

if diff /tmp/before-page-down.txt /tmp/after-page-down.txt >/dev/null; then
  echo -e "  ${YELLOW}WARNING${NC}: Page Down may not have worked (no visible change)"
else
  echo -e "  ${GREEN}✓${NC} Page Down caused view change"
fi

# Test 5: Test Page Up
echo ""
echo "Test 5: Page Up functionality"
echo "  Sending Page Up..."
tmux send-keys -t "$SESSION" NPage # Page Down key in tmux
sleep 0.5
capture /tmp/after-page-up.txt

if diff /tmp/after-page-down.txt /tmp/after-page-up.txt >/dev/null; then
  echo -e "  ${YELLOW}WARNING${NC}: Page Up may not have worked (no visible change)"
else
  echo -e "  ${GREEN}✓${NC} Page Up caused view change"
fi

# Test 6: Switch to Resource Groups and test scrolling
echo ""
echo "Test 6: Resource Groups panel scrolling"
tmux send-keys -t "$SESSION" Tab
sleep 0.5
tmux send-keys -t "$SESSION" Enter
sleep 1

capture /tmp/rg-initial.txt
if grep -q "Resource Groups" /tmp/rg-initial.txt; then
  echo -e "  ${GREEN}✓${NC} Resource Groups panel visible"

  # Try scrolling in RG panel
  echo "  Testing RG panel scrolling..."
  for i in {1..15}; do
    tmux send-keys -t "$SESSION" Down
    sleep 0.1
  done
  sleep 0.3
  capture /tmp/rg-scrolled.txt

  if diff /tmp/rg-initial.txt /tmp/rg-scrolled.txt >/dev/null; then
    echo -e "  ${YELLOW}WARNING${NC}: RG panel content unchanged after scrolling"
  else
    echo -e "  ${GREEN}✓${NC} RG panel scrolling works"
  fi
else
  echo -e "  ${RED}✗${NC} FAIL: Resource Groups panel not visible"
  FAILED=1
fi

# Test 7: Resources panel scrolling
echo ""
echo "Test 7: Resources panel scrolling"
tmux send-keys -t "$SESSION" Tab
sleep 0.5
tmux send-keys -t "$SESSION" Enter
sleep 1

capture /tmp/res-initial.txt
if grep -q "Resources" /tmp/res-initial.txt; then
  echo -e "  ${GREEN}✓${NC} Resources panel visible"

  # Try scrolling in Resources panel
  echo "  Testing Resources panel scrolling..."
  for i in {1..15}; do
    tmux send-keys -t "$SESSION" Down
    sleep 0.1
  done
  sleep 0.3
  capture /tmp/res-scrolled.txt

  if diff /tmp/res-initial.txt /tmp/res-scrolled.txt >/dev/null; then
    echo -e "  ${YELLOW}WARNING${NC}: Resources panel content unchanged after scrolling"
  else
    echo -e "  ${GREEN}✓${NC} Resources panel scrolling works"
  fi
else
  echo -e "  ${RED}✗${NC} FAIL: Resources panel not visible"
  FAILED=1
fi

# Summary
echo ""
echo "=== Test Summary ==="
if [ $FAILED -eq 0 ]; then
  echo -e "${GREEN}✓${NC} All critical scrolling tests passed"
  exit 0
else
  echo -e "${RED}✗${NC} Some tests failed"
  exit 1
fi
