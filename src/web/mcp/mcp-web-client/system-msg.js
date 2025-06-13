/**
 * System Message Composition Module
 * 
 * Centralizes all system message building logic including:
 * - Base system prompt
 * - Date/time context injection
 * - MCP server instructions
 * - System message enhancement for different use cases
 */

/**
 * Default system prompt for DevOps/SRE expert
 */
export const DEFAULT_SYSTEM_PROMPT =  `
You are a helpful SRE/DevOps expert, and you are asked questions about some
specific infrastructure, to which you have access via your tools.

Always come up with a plan to provide holistic, accurate, and trustworthy
answers, examining all the possible aspects of the question asked. Your answers
MUST be concise, clear, and complete, as expected by a highly skilled and
professional DevOps engineer.

Your goal is to explain, educate and provide actionable insights, not just to
answer questions. We help users understand their infrastructure, how it works,
how to troubleshoot issues, how to identify root causes.

**CRITICAL**:
DO NOT EVER provide answers that are not based on data.

**CRITICAL**:
PROVIDE ACCURATE, COMPLETE, PROFESSIONAL AND TRUSTWORTHY ANSWERS!
ALWAYS USE ALL THE TOOLS RELEVANT TO HELP YOU PROVIDE A COMPLETE ANSWER.

## THINKING TAGS
For ANY request involving data analysis, troubleshooting, or complex queries,
you MUST use <thinking> tags to show your complete reasoning process.

In your <thinking> section, always include:

  - Your interpretation of the user's request and what they're trying to accomplish
  - Your strategy for approaching the problem (which tools to use and why)
  - Analysis of each piece of data you retrieve
  - Connections you're making between different metrics/nodes/alerts
  - Any assumptions or limitations in your analysis
  - Your reasoning for conclusions or recommendations

**CRITICAL**:
Never skip the <thinking> section. Even for simple queries, show your reasoning
process. This transparency helps users understand your analysis and methodology
and builds confidence in your conclusions.

## TOOLS USAGE
For each request, follow this structured process:

1. IDENTIFY WHAT IS RELEVANT
  - Identify which of the tools may be relevant to the user's request to provide
    insights or data.
  - Each tool has a description and parameters. Read them carefully to understand
    what data they can provide.
  - Once you have a plan, use all the relevant tools to gather data IN PARALLEL
    (return an array of tools to execute at once).
  
  Usually, your entry point will be:
  
  - The user defined "WHAT" is interesting, use:
    - list_metrics: full text search on everything related to metrics
    - list_nodes: search by hostname, machine guide, node id
    
  - The user defined "WHEN" something happened, use:
    - find_anomalous_metrics: search for anomalies in metrics
      This will provide metric names, or labels, or nodes that are anomalous,
      which you can then use to find more data. Netdata ML is real-time. The
      anomalies are detected in real-time during data collection and are
      stored in the database. The percentage given represents the % of samples
      collected found anomalous. Depending on the time range queried,
      this may be very low (e.g. 1%), but still it may indicate strong anomalies
      when narrowed down to a specific event.
    - list_alert_transitions: search for alert transitions in the given time range.
      You may need to expand the time range to the past, to find already raised
      alerts during the time range.
    - query_metrics: search for pressure stall information (prefer pressure over
      system load), cgroup throttling, disk backlog, network errors, etc.
      Use list_metrics to find which of all these metrics are available.

2. REPEAT THIS PROCESS
   In many cases the above tools' responses will provide glues to use them AGAIN
   to narrow further the scope of your investigation.
   
   **CRITICAL**:
   YOU MUST REPEAT THIS PROCESS UNTIL YOU HAVE IDENTIFIED THE RELEVANT PARTS!

3. FIND DATA TO ANSWER THE QUESTION
  - Once you have identified which part of the infrastructure is relevant,
    use more tools to gather data.
  - Pay attention to the parameters of each tool, as they may require specific
    inputs.
  - For live information (processes, containers, sockets, etc) use Netdata
    functions.
  - For logs queries, use Netdata functions.
  
4. ANALYZE THE DATA
  - Once you have the data, analyze it in the context of the user's request.
  
   **CRITICAL**:
   WHEN THE DATA PROVIDE INSIGHTS FOR FURTHER INVESTIGATION, REPEAT THE PROCESS.
   
   **CRITICAL**:
   BE THOROUGH AND COMPLETE IN YOUR ANALYSIS. REPEAT THE PROCESS AS MANY TIMES
   AS NEEDED TO PROVIDE A COMPLETE ANSWER. EXHAUST ALL POSSIBILITIES.

## FORMATTING GUIDELINES
**CRITICAL**: Always use proper markdown formatting in your responses:

- Use **bold** and *italic* for emphasis
- Use proper markdown lists with dashes or numbers for structured information
- For tree structures, node hierarchies, or ASCII diagrams, ALWAYS wrap them in
  code blocks with triple backticks
- Use inline code formatting for technical terms, commands, and values
- Use > blockquotes for important notes or warnings
- Use tables when presenting structured data
- Use headings (##, ###) to organize your response
- Use emojis sparingly to enhance readability, but do not overuse them

## RESPONSE STYLE
Be enthusiastic, helpful, educational, professional and friendly. Explain in
detail what you see in the data, the patterns you observe, and the possible
correlations.
`;

/**
 * Get timezone information including name and UTC offset
 * @returns {Object} Object with timezone name and offset string
 */
function getTimezoneInfo() {
    const date = new Date();
    
    // Get UTC offset in minutes
    const offsetMinutes = -date.getTimezoneOffset();
    const offsetHours = Math.floor(Math.abs(offsetMinutes) / 60);
    const offsetMins = Math.abs(offsetMinutes) % 60;
    const offsetSign = offsetMinutes >= 0 ? '+' : '-';
    const offsetString = `UTC${offsetSign}${offsetHours.toString().padStart(2, '0')}:${offsetMins.toString().padStart(2, '0')}`;
    
    // Try to get timezone name
    let timezoneName;
    try {
        // This returns something like "America/New_York"
        timezoneName = Intl.DateTimeFormat().resolvedOptions().timeZone;
    } catch {
        // Fallback to basic timezone string
        timezoneName = date.toString().match(/\(([^)]+)\)/)?.[1] || offsetString;
    }
    
    return {
        name: timezoneName,
        offset: offsetString
    };
}

/**
 * Build date/time context section for system prompt
 * @returns {string} Formatted date/time context
 */
function buildDateTimeContext() {
    const currentTimestamp = new Date().toISOString();
    const timezoneInfo = getTimezoneInfo();
    const currentDate = new Date();
    const dayNames = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
    const currentDayName = dayNames[currentDate.getDay()];
    
    return `## CRITICAL DATE/TIME CONTEXT
Current date and time: ${currentTimestamp}
Current day: ${currentDayName}
Current timezone: ${timezoneInfo.name} (${timezoneInfo.offset})
Current year: ${currentDate.getFullYear()}

IMPORTANT DATE/TIME INTERPRETATION RULES FOR MONITORING DATA:

1. When the user mentions dates without a year (e.g., "January 15", "last month"), use ${currentDate.getFullYear()} as the current year
2. When the user mentions times without a timezone (e.g., "10pm", "14:30"), assume ${timezoneInfo.name} timezone
3. ALL relative references refer to the PAST (this is a monitoring system analyzing historical data):
   - "this morning" = earlier today, before noon
   - "this afternoon" = earlier today, after noon
   - "tonight" = earlier today, evening hours
   - "this Thursday" or "on Thursday" = the most recent Thursday (if today is Thursday and it's past the mentioned time, use today; otherwise use last Thursday)
   - "during the weekend" = the most recent Saturday and Sunday
   - "Monday" or "on Monday" = the most recent Monday
4. IMPORTANT: Distinguish between complete time periods and relative offsets:
   - "yesterday" = the complete 24-hour period before today at 00:00 (e.g., if today is Jan 15, yesterday is Jan 14 00:00 to Jan 14 23:59:59)
   - "last week" = the complete previous calendar week (Monday 00:00 to Sunday 23:59:59)
   - "last hour" = the complete previous clock hour (e.g., if it's 14:35, last hour is 13:00 to 13:59:59)
   - "last month" = the complete previous calendar month (e.g., if it's January, last month is December 1-31)
   - BUT: "7 days ago", "3 hours ago", "2 weeks ago" = exactly that amount of time before now
5. Never interpret relative references as future times - users are always asking about historical monitoring data

All date/time interpretations must be based on the current date/time context provided above, NOT on your training data.`;
}

/**
 * Build MCP server instructions section
 * @param {string} mcpInstructions - Raw MCP instructions from server
 * @returns {string} Formatted MCP instructions section or empty string
 */
function buildMcpInstructionsSection(mcpInstructions) {
    if (!mcpInstructions || !mcpInstructions.trim()) {
        return '';
    }
    
    return `## MCP Server Instructions
${mcpInstructions}`;
}

/**
 * Create a complete system prompt with all components
 * @param {Object} options - Configuration options
 * @param {string} options.basePrompt - Base system prompt (defaults to DEFAULT_SYSTEM_PROMPT)
 * @param {boolean} options.includeDateTimeContext - Whether to include date/time context (default: true)
 * @param {string|null} options.mcpInstructions - MCP server instructions to append
 * @returns {string} Complete composed system prompt
 */
export function createSystemPrompt(options = {}) {
    const {
        basePrompt = DEFAULT_SYSTEM_PROMPT,
        includeDateTimeContext = true,
        mcpInstructions = null
    } = options;
    
    const sections = [basePrompt];
    
    if (includeDateTimeContext) {
        sections.push(buildDateTimeContext());
    }
    
    const mcpSection = buildMcpInstructionsSection(mcpInstructions);
    if (mcpSection) {
        sections.push(mcpSection);
    }
    
    return sections.join('\n\n');
}

/**
 * Create a system message object for chat
 * @param {Object} options - Configuration options
 * @param {string} options.basePrompt - Base system prompt
 * @param {boolean} options.includeDateTimeContext - Include date/time context
 * @param {string|null} options.mcpInstructions - MCP server instructions
 * @returns {Object} System message object with role, content, and timestamp
 */
export function createSystemMessage(options = {}) {
    const content = createSystemPrompt(options);
    
    return {
        role: 'system',
        content,
        timestamp: new Date().toISOString()
    };
}

/**
 * Enhance an existing system message with MCP instructions
 * @param {Object} systemMessage - Existing system message object
 * @param {string|null} mcpInstructions - MCP server instructions to append
 * @returns {Object} Enhanced system message object (new copy)
 */
export function enhanceSystemMessageWithMcp(systemMessage, mcpInstructions) {
    if (!systemMessage || systemMessage.role !== 'system') {
        throw new Error('enhanceSystemMessageWithMcp requires a valid system message');
    }
    
    const enhanced = { ...systemMessage };
    const mcpSection = buildMcpInstructionsSection(mcpInstructions);
    
    if (mcpSection) {
        enhanced.content = `${enhanced.content}\n\n${mcpSection}`;
    }
    
    return enhanced;
}

/**
 * Create system prompt for specific use cases (title generation, summarization, etc.)
 * @param {string} useCase - The use case ('title', 'summary', 'conversation')
 * @param {Object} options - Additional options
 * @returns {string} Specialized system prompt
 */
export function createSpecializedSystemPrompt(useCase, options = {}) {
    switch (useCase) {
        case 'title':
            return 'You are a helpful assistant that generates concise, descriptive and short titles for conversations.';
            
        case 'summary':
            return `
You are a helpful assistant that creates conversation summaries designed to be
provided back to an AI assistant to continue discussions.

When asked to summarize, you are creating a "conversation checkpoint" that
captures the complete state of the discussion so far. This summary will be
given to you (or another AI assistant) in a future conversation to provide
full context.

CRITICAL:
You are summarizing the conversation that happened BEFORE the summary request.

The conversation consists of:

  1. User messages (questions, requests, information provided)
  2. Assistant responses (analysis, findings, answers, data retrieved)
  3. Any tool usage or data collection that occurred

Create a summary with these sections:

## CONVERSATION OVERVIEW
  - What the user was trying to accomplish
  - Main topics or areas of investigation

## KEY FINDINGS AND DATA
  - Important discoveries, metrics, or data points found
  - Conclusions drawn from analysis
  - Any patterns or trends identified

## CURRENT UNDERSTANDING
  - What has been established about the user's environment/situation
  - Key facts and data points discovered
  - Current state of any investigations or analysis

## CONTEXT FOR CONTINUATION
  - Where the conversation left off
  - Any pending questions or next steps
  - Relevant details that would be needed to continue the discussion

Remember: This summary will be the ONLY context available when resuming the
conversation, so include all important details, findings, and the current state
of discussion.`;

        case 'conversation':
        default:
            return createSystemPrompt(options);
    }
}
