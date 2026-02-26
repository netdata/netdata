# Documentation Tester Enhancement - Workflow Testing

## Problem Statement

The current documentation tester only tests **code blocks** (commands, configurations, API calls) but doesn't follow **narrative instructions** and **multi-step workflows** described in prose.

This is a critical gap because:
- Documentation often describes procedures across multiple paragraphs
- Users must follow step-by-step instructions
- The success of documentation is measured by whether the entire workflow works

## Current Capabilities

### What Works:
```python
# Tests only code blocks
```bash
systemctl restart netdata
```

✅ Parse: Extracts bash command
✅ Test: Executes command on VM
✅ Validate: Checks exit code
```

### What's Missing:
```markdown
# Narrative instructions (not tested)

## To enable badge restrictions

1. Edit `/etc/netdata/netdata.conf`
2. Add `[web]` section
3. Restart Netdata
4. Verify restrictions work

❌ Parser: Doesn't extract these steps
❌ Executor: Doesn't follow instructions
❅ Validator: Doesn't verify end-to-end workflow
```

## Proposed Enhancement

### 1. Procedural Extraction

Extract step-by-step instructions from text:

```python
def extract_workflows(text: str) -> List[Workflow]:
    """
    Identify procedural instructions:
    - Numbered lists (1., 2., 3.)
    - Step markers (First, Next, Finally)
    - Task sequences ("To do X:", "Follow these steps:")
    """
    patterns = [
        r'(?:^|\n)(\d+)[.)\s+(.+)',  # Numbered lists
        r'(?i)first.?\s+(.+)',            # First step
        r'(?i)next.?\s+(.+)',               # Next step
        r'(?i)then.?\s+(.+)',              # Then do this
        r'(?i)finally.?\s+(.+)',            # Finally step
    ]
    
    workflows = []
    current_workflow = Workflow()
    
    for line in text.split('\n'):
        if step := find_pattern(line, patterns):
            current_workflow.add_step(step)
        elif is_new_section(line):
            if current_workflow.steps:
                workflows.append(current_workflow)
            current_workflow = Workflow()
    
    return workflows
```

### 2. Workflow Execution

Follow each step and verify success:

```python
class WorkflowExecutor:
    def execute_workflow(self, workflow: Workflow) -> TestResult:
        """
        Execute a multi-step workflow and verify each step
        """
        result = {
            'type': 'workflow',
            'description': workflow.description,
            'steps': [],
            'status': 'PASS',
            'evidence': []
        }
        
        for step in workflow.steps:
            step_result = self.execute_step(step)
            result['steps'].append(step_result)
            result['evidence'].extend(step_result['evidence'])
            
            if not step_result['success']:
                result['status'] = 'FAIL'
                result['failed_at_step'] = step.number
                result['error'] = step_result['error']
                break  # Stop on failure
        
        return result
    
    def execute_step(self, step: Step) -> StepResult:
        """
        Execute a single step from workflow instructions
        """
        if step.type == 'command':
            return self.execute_command(step)
        elif step.type == 'file_operation':
            return self.execute_file_operation(step)
        elif step.type == 'verification':
            return self.verify_state(step)
        elif step.type == 'wait':
            return self.wait_condition(step)
```

### 3. Step Types to Support

#### Command Execution
```markdown
**Step 3: Restart Netdata**

```bash
sudo systemctl restart netdata
```
```
```python
step = {
    'type': 'command',
    'instruction': 'sudo systemctl restart netdata',
    'expected': 'Service restarts successfully'
}
```

#### File Operations
```markdown
**Step 1: Create configuration file**

Create `/etc/netdata/my-config.conf` with:
```ini
[web]
    allow badges from = 10.*
```
```
```python
step = {
    'type': 'file_operation',
    'action': 'create',
    'path': '/etc/netdata/my-config.conf',
    'content': '[web]\n    allow badges from = 10.*',
    'expected': 'File exists with correct content'
}
```

#### Verification Steps
```markdown
**Step 4: Verify restrictions work**

Test that accessing from blocked IP fails:
```
```
```python
step = {
    'type': 'verification',
    'check': 'badge_access_restricted',
    'condition': 'blocked_ip_returns_error',
    'expected': 'Access denied from non-allowed IP'
}
```

#### Wait Conditions
```markdown
**Step 3: Wait for service to stabilize**

Wait 10 seconds for Netdata to fully start.
```
```python
step = {
    'type': 'wait',
    'duration': 10,
    'condition': 'service_running',
    'expected': 'Netdata service is active'
}
```

### 4. Workflow Examples

#### Example 1: Enable Badge Restrictions

**Documentation:**
```markdown
## Restricting Badge Access

1. Edit `/etc/netdata/netdata.conf`
2. Add `[web]` section
3. Set `allow badges from = 10.*`
4. Restart Netdata
5. Verify restrictions work
```

**Enhanced Tester Output:**
```markdown
### ❌ FAIL: Workflow - Restricting Badge Access (Lines 10-15)

**What was tested**:
- Step 1: Edited /etc/netdata/netdata.conf ✅
- Step 2: Added [web] section ✅
- Step 3: Set allow badges from = 10.* ✅
- Step 4: Restarted Netdata ✅
- Step 5: Verified restrictions ❌

**Evidence**:
```
Step 1: File edited successfully
sudo nano /etc/netdata/netdata.conf
[web] section added
allow badges from = 10.* added

Step 4: Service restarted
sudo systemctl restart netdata
Job for netdata.service finished

Step 5: Verification failed
curl http://localhost:19999/api/v1/badge.svg?chart=system.cpu
# Returned badge successfully (should have been blocked)
```

**Result**: FAIL
**Error**: Step 5 failed - badges were not restricted as documented
**Issue**: Documentation claim that "restrictions work" is incorrect
```

#### Example 2: Multi-Step Account Deletion

**Documentation:**
```markdown
## Deleting Your Account

1. Log in to your dashboard
2. Navigate to Settings > Account
3. Click "Delete Account"
4. Confirm deletion
5. Verify account is deleted
```

**Enhanced Tester Output:**
```markdown
### ✅ PASS: Workflow - Deleting Account (Lines 20-25)

**What was tested**:
- Step 1: Navigated to dashboard ✅
- Step 2: Found Settings > Account ✅
- Step 3: Clicked "Delete Account" ✅
- Step 4: Confirmed deletion ✅
- Step 5: Verified account deleted ✅

**Evidence**:
```
Step 1: Login successful
curl -X POST https://cloud.netdata.cloud/api/login
{"status": "success"}

Step 2: Settings page loaded
curl https://cloud.netdata.cloud/settings/account
200 OK

Step 5: Account no longer exists
curl https://cloud.netdata.cloud/api/account/123
404 Not Found
```

**Result**: PASS
**Note**: All steps completed successfully
```

### 5. Implementation Plan

#### Phase 1: Procedural Extraction
- [ ] Implement workflow pattern matching
- [ ] Extract numbered list steps
- [ ] Extract "First/Next/Finally" markers
- [ ] Extract "To do X:" patterns

#### Phase 2: Step Classification
- [ ] Detect command steps
- [ ] Detect file operations
- [ ] Detect verification checks
- [ ] Detect wait conditions
- [ ] Detect navigation steps (UI testing)

#### Phase 3: Execution Engine
- [ ] Implement command executor
- [ ] Implement file operations (create/edit/delete)
- [ ] Implement verification checks
- [ ] Implement wait conditions
- [ ] Implement UI navigation (if applicable)

#### Phase 4: State Management
- [ ] Track execution state
- [ ] Store evidence for each step
- [ ] Support rollback/cleanup on failure
- [ ] Generate workflow reports

#### Phase 5: Testing
- [ ] Test on existing workflows
- [ ] Test on edge cases
- [ ] Verify rollback behavior
- [ ] Test error handling

### 6. Testing Scenarios

#### Scenario 1: Configuration Change Workflow
```markdown
1. Backup current config
2. Edit configuration
3. Apply changes
4. Restart service
5. Verify service is running
6. Verify new settings work
```

#### Scenario 2: Installation Workflow
```markdown
1. Download installation script
2. Make script executable
3. Run script with sudo
4. Verify installation completed
5. Check service status
```

#### Scenario 3: Troubleshooting Workflow
```markdown
1. Identify problem
2. Run diagnostic command
3. Check log files
4. Apply fix
5. Verify fix worked
```

### 7. Integration with Current Tester

Combine code-block testing with workflow testing:

```python
class EnhancedDocumentationTester(DocumentationTester):
    def test_documentation(self, file_path: str) -> TestReport:
        """
        Test documentation with both code blocks and workflows
        """
        report = TestReport()
        
        # Test code blocks (existing)
        claims = self.parse_code_blocks(file_path)
        for claim in claims:
            result = self.test_claim(claim)
            report.add_result(result)
        
        # Test workflows (new)
        workflows = self.extract_workflows(file_path)
        for workflow in workflows:
            result = self.execute_workflow(workflow)
            report.add_result(result)
        
        return report
```

### 8. Priority Features

#### High Priority:
1. **Numbered list extraction** - Most docs use this format
2. **Command execution** - Already partially implemented
3. **File operations** - Critical for configuration docs

#### Medium Priority:
4. **Verification steps** - Test that conditions are met
5. **Wait conditions** - Support timing-dependent steps
6. **Evidence capture** - Store output/state for each step

#### Low Priority:
7. **UI navigation** - For web-based documentation
8. **Multi-branch testing** - Test alternative approaches
9. **Conditional steps** - Handle "if X then Y" logic

### 9. Output Format

Enhanced test reports with workflow results:

```markdown
## Documentation Test Report

### Code Block Tests

✅ PASS: Configuration example (Line 10-20)
❌ FAIL: Command example (Line 30-35)

### Workflow Tests

✅ PASS: Workflow - Enable badge restrictions (Lines 40-55)
- Steps executed: 5/5
- Failed at: Step 5 (Verification)
- Evidence: Attached

❌ FAIL: Workflow - Account deletion (Lines 60-75)
- Steps executed: 2/5
- Failed at: Step 3 (Click "Delete Account")
- Reason: Button not found on page
```

### 10. Limitations and Risks

#### Limitations:
- UI workflows harder to automate
- Conditional logic requires manual interpretation
- Multi-user scenarios may conflict with test environment

#### Mitigations:
- Mark UI workflows as "requires manual verification"
- Provide screenshots for UI steps
- Test in isolated environment
- Support dry-run mode for destructive operations

### 11. Example: Testing PR #21711

Current approach only tested:
- Code blocks (bash commands, configs)
- API endpoints

Enhanced approach would also test:
- Workflow: Enable badge access restriction
  1. Edit netdata.conf
  2. Add [web] section
  3. Set allow badges from parameter
  4. Restart Netdata
  5. Verify restrictions work

This would catch the actual issue found in the PR: the workflow doesn't work because [web] section doesn't exist in default installations.

### 12. Conclusion

The documentation tester must evolve from:
- **Code syntax validation** → **Workflow execution validation**

This shift ensures that:
- Users can actually follow the instructions
- Multi-step procedures work end-to-end
- Documentation claims are tested in realistic scenarios

**Success metric**: Not just "does the code run?" but "can a user complete the task by following this documentation?"

---

**Next Steps:**
1. Review current documentation to identify common workflow patterns
2. Prioritize implementation based on documentation type (Agent vs Cloud vs UI)
3. Start with numbered list extraction (highest value)
4. Incrementally add step types
5. Test on existing documentation files
