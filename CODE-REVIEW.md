# Code Review Prompt Template

## Review Objective
Conduct a comprehensive, line-by-line code review with deep understanding of functionality, architecture, and potential edge cases. This review must go beyond surface-level completeness checking to ensure code quality, correctness, and architectural integrity.

## Pre-Review Analysis

### 1. Specification Alignment Check
- **Primary Objective**: Verify every change directly implements the stated requirements
- **Questions to Answer**:
  - Does each line of code serve the explicit purpose stated in the requirements?
  - Are there any changes that implement unspecified functionality?
  - Do all changes directly support the core objective described in the spec?
- **Action**: Map each change to its corresponding requirement

### 2. Change Impact Assessment
- **Primary Objective**: Identify all dependencies and potential ripple effects
- **Analysis Required**:
  - Direct dependencies: What functions/classes rely on the changed code?
  - Indirect dependencies: What other systems might be affected?
  - Data flow changes: How do modifications affect input/output contracts?
  - State management: Are there any shared mutable states affected?
- **Action**: Create a dependency map before deeper review

## Line-by-Line Review Process

### Step 1: Syntax and Logic Verification
- **Line-by-line examination**: Read every modified line with surgical precision
- **Logic validation**: Ensure the code logic matches the intended behavior
- **Edge case consideration**: What happens with null values, empty inputs, extreme values?
- **Type safety**: Are all type assumptions valid and safe?

### Step 2: Specification Compliance Check
- **Requirement mapping**: Does each change directly address the specified requirements?
- **Unwanted functionality**: Are there any implementations that go beyond scope?
- **Missing requirements**: What specified functionality is not implemented?

### Step 3: Architecture and Design Review
- **Design patterns**: Do changes follow established architectural patterns?
- **Separation of concerns**: Are concerns properly separated and cohesive?
- **Coupling assessment**: Are changes appropriately decoupled from other components?
- **Responsibility boundaries**: Does each component have clear, single responsibilities?

## Flow and Control Logic Analysis

### When Control Flow Changes Are Detected:
1. **Complete Function Review**: Examine the entire function, not just modified lines
2. **Original Functionality Preservation**: 
   - Can you trace through the entire flow to ensure no regression?
   - What was the function's original behavior, and is it maintained?
   - Are there any paths that could break due to the changes?
3. **New Flow Validation**:
   - Does the new flow logic make sense and handle all scenarios?
   - Are there potential infinite loops or dead code paths?
   - Does error handling cover all failure modes?

### Edge Case Discovery Process:
1. **Input space analysis**: Consider minimum, maximum, empty, null, and boundary conditions
2. **Sequence analysis**: "What happens if A happens, then B happens, then C happens?"
3. **State interaction**: How do changes affect shared state between components?
4. **Timing considerations**: Are there race conditions or ordering dependencies?
5. **Resource constraints**: Memory leaks, performance degradation, resource exhaustion?

## Regression Risk Assessment

### Functionality Regression Check:
- **Original feature preservation**: Does each change maintain existing functionality?
- **Side effect analysis**: What other features might be affected indirectly?
- **Integration points**: How do changes affect other system components?
- **API contract changes**: Are breaking changes properly handled?

### Performance Impact:
- **Algorithm complexity**: Are there unnecessary O(n²) operations where O(log n) exists?
- **Memory usage**: Could changes lead to memory leaks or excessive allocation?
- **Concurrency issues**: Are there potential race conditions or deadlock scenarios?

## Design Quality Assessment

### Code Smell Detection:
- **Over-engineering**: Is the solution more complex than necessary?
- **Under-engineering**: Are there obvious simplifications or improvements missed?
- **Duplication**: Is there repeated logic that could be consolidated?
- **Cohesion vs Coupling**: Are related concerns properly grouped?

### Maintainability Review:
- **Readability**: Is the code self-documenting with clear variable/function names?
- **Testability**: Can the changes be easily unit tested?
- **Documentation**: Is the complexity adequately commented?
- **Future extensibility**: How easy will it be to modify this code later?

## Review Checklist

### Critical Review Questions:
1. **Correctness**: Is this change actually correct for the stated requirement?
2. **Completeness**: Does it handle all specified requirements and edge cases?
3. **Consistency**: Does it match existing code style, patterns, and conventions?
4. **Architecture**: Does it maintain or improve the overall system architecture?
5. **Performance**: Could it introduce performance regressions?
6. **Security**: Are there potential security vulnerabilities introduced?
7. **Maintainability**: Will future developers understand and be able to modify this?

### Edge Cases to Consider:
- **Boundary conditions**: Min/max values, empty collections, null inputs
- **Error scenarios**: What happens when external dependencies fail?
- **Concurrency issues**: Race conditions, deadlocks, inconsistent states
- **Resource constraints**: Memory, CPU, network, disk space limitations
- **Integration failures**: What happens when dependent services are unavailable?

## Review Output Requirements

### Summary Structure:
1. **High-Level Assessment**: Overall quality and spec alignment
2. **Critical Issues**: Any problems that must be fixed before merge
3. **Function-by-Function Analysis**: For each changed function
4. **Edge Cases Identified**: Potential failure modes discovered
5. **Architectural Concerns**: Broader system impact assessment
6. **Recommendations**: Suggestions for improvement or alternative approaches

### Depth Requirements:
- **Minimum**: Every changed line must have comment explaining its purpose
- **Edge Cases**: At least 3-5 edge cases identified per modified function
- **Architecture**: Clear understanding of how changes fit into broader system design
- **Risk Assessment**: Specific risks identified with mitigation strategies

## Final Approval Criteria

A change set is ready for merge only if:
- ✅ Every line maps to a specific requirement
- ✅ No unintended functionality or side effects
- ✅ Original functionality fully preserved
- ✅ Edge cases identified and handled
- ✅ Architecture remains sound
- ✅ Performance impact acceptable
- ✅ Security considerations addressed
- ✅ Code quality meets project standards

**Remember**: The goal is not just to find bugs, but to understand the essence of each function and ensure the implementation is not just correct, but well-designed and maintainable.