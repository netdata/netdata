#!/bin/bash
# Complete PR Testing Workflow
# This script demonstrates the full workflow: test docs -> comment on PR

set -e

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

if [ $# -lt 1 ]; then
    echo "Usage: $0 <PR_NUMBER> [documentation_file]"
    echo ""
    echo "Example:"
    echo "  $0 21806                                    # Auto-find modified docs"
    echo "  $0 21806 docs/netdata-conf.md                # Test specific file"
    exit 1
fi

PR_NUMBER=$1
DOC_FILE=$2

echo -e "${BLUE}══════════════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}   DOCUMENTATION TESTER - PR WORKFLOW${NC}"
echo -e "${BLUE}══════════════════════════════════════════════════════════════════${NC}"
echo ""

# Step 1: Get PR information
echo -e "${YELLOW}[Step 1/5] Fetching PR information...${NC}"
PR_INFO=$(gh pr view $PR_NUMBER --json title,files --jq '{title: .title, files: [.files[].path]}')

echo "PR #$PR_NUMBER: $(echo $PR_INFO | jq -r '.title')"
echo ""

# Step 2: Find modified documentation files
if [ -z "$DOC_FILE" ]; then
    echo -e "${YELLOW}[Step 2/5] Finding modified documentation files...${NC}"
    DOC_FILES=$(echo $PR_INFO | jq -r '.files[] | select(startswith("docs/") or contains("README.md"))')
    
    if [ -z "$DOC_FILES" ]; then
        echo -e "${RED}No documentation files found in this PR${NC}"
        exit 0
    fi
    
    echo "Found documentation files:"
    echo "$DOC_FILES" | nl
    echo ""
    
    # Use the first doc file (or ask user)
    DOC_FILE=$(echo "$DOC_FILES" | head -1)
    echo -e "${GREEN}Selected: $DOC_FILE${NC}"
    echo ""
else
    echo -e "${YELLOW}[Step 2/5] Using specified documentation file...${NC}"
    echo -e "${GREEN}File: $DOC_FILE${NC}"
    echo ""
fi

# Step 3: Run documentation tests
echo -e "${YELLOW}[Step 3/5] Running documentation tests...${NC}"

# Ensure we're in the right directory
cd /Users/kanela/src/netdata/ai-agent

# Run the tester
echo "Testing: $DOC_FILE"
TESTER_OUTPUT=$(python3 netdata_tester/docs_tester/tester.py "$DOC_FILE" 2>&1)
echo "$TESTER_OUTPUT"

# Extract report file from output
REPORT_FILE=$(echo "$TESTER_OUTPUT" | grep "Report saved to:" | awk '{print $3}')

if [ -z "$REPORT_FILE" ]; then
    echo -e "${RED}Failed to find report file${NC}"
    exit 1
fi

echo -e "${GREEN}Report generated: $REPORT_FILE${NC}"
echo ""

# Step 4: Preview the comment
echo -e "${YELLOW}[Step 4/5] Previewing PR comment...${NC}"
echo ""
python3 netdata_tester/docs_tester/pr_commenter.py "$REPORT_FILE" $PR_NUMBER --dry-run
echo ""

# Step 5: Post to PR
read -p "Post this comment to PR #$PR_NUMBER? (y/n): " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${YELLOW}[Step 5/5] Posting comment to PR #$PR_NUMBER...${NC}"
    python3 netdata_tester/docs_tester/pr_commenter.py "$REPORT_FILE" $PR_NUMBER
    echo ""
    echo -e "${GREEN}✓ Comment posted successfully!${NC}"
    echo "View: https://github.com/netdata/netdata/pull/$PR_NUMBER"
else
    echo -e "${YELLOW}Comment posting skipped${NC}"
    echo ""
    echo "You can post manually with:"
    echo "  python3 netdata_tester/docs_tester/pr_commenter.py \"$REPORT_FILE\" $PR_NUMBER"
fi

echo ""
echo -e "${BLUE}══════════════════════════════════════════════════════════════════${NC}"
echo -e "${GREEN}Workflow complete!${NC}"
echo -e "${BLUE}══════════════════════════════════════════════════════════════════${NC}"
