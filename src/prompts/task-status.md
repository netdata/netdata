#### agent__task_status — Task Status Feedback

**Purpose:** This tool provides live feedback to the user about your accomplishments, pendings and immediate goals related to the task assigned to you. It is purely for REPORTING. It does not perform any actions and it does not help in completing your task. Use it ONLY to communicate your progress to the user, while you work on the assigned task.

All its fields are MANDATORY:

Field `status`: indicates your current state in the task:
- "starting": You just started and you are planning your actions
- "in-progress": You are currently working on this task - you are not yet ready to provide your final report/answer
- "completed": You completed the task and you are now ready to provide your final report/answer

Field `done`: A brief summary of what you have accomplished so far:
- Describe which parts of the task you have completed - not the tools you run - the user cares about results, not methods.
- Focus on outcomes and findings relevant to the task.

Field `pending`: A brief summary of what is still pending or left to do to complete the task:
- Describe which parts of the task are still pending or need further work.
- Focus on outstanding items that are necessary to complete the task.

Field `now`: A brief description of the actions you are taking IN THIS TURN:
- Describe what tools you are calling alongside this status update
- Explain what you are trying to achieve with your current actions
- If you are providing your final report, state that here

Field `ready_for_final_report`:
- Set to true when you have enough information to provide your final report/answer
- Set to false when you still need to gather more information or perform more actions

Field `need_to_run_more_tools`:
- Set to true when you need to run more tools
- Set to false if you are done with tools

What to report when tools fail:
- Be honest: if you are facing difficulties or limitations, clearly state them in the `done` field.
- Try to work around failures: explain in the `now` field what alternative steps you are taking to overcome the issues.
- If you tried everything and you still cannot proceed to complete the task, provide your final report/answer, honestly stating the limitations you faced.

**WRONG (wastes a turn):**
- Calling agent__task_status with now="Searching for more data" without calling any data retrieval tool

**RIGHT:**
- Calling agent__task_status TOGETHER with other tools in the same turn

**Good Examples:**
- status: "starting", done: "Planning...", pending: "Find error logs", now: "gather system error logs for the last 15 mins", ready_for_final_report: false, need_to_run_more_tools: true, and at the same time executing the right tools for log retrieval
- status: "in-progress", done: "got error logs for the last 15 mins", pending: "Find the specific error", now: "expand search to 30 mins", ready_for_final_report: false, need_to_run_more_tools: true, and at the same time executing the right tools for expanded log retrieval
- status: "in-progress", done: "Found relevant logs", pending: "Identify root cause", now: "Examining source code", ready_for_final_report: true, need_to_run_more_tools: true, and at the same time executing the right tools for code analysis
- status: "completed", done: "Found 3 critical errors", pending: "All done", now: "Compile the final report/answer", ready_for_final_report: true, need_to_run_more_tools: false, and at the same time providing the final report/answer

**Best Practices:**
- Call agent__task_status alongside other tools or your final report/answer (calling it alone wastes turns)
- Include clear descriptions for "done", "pending" and "now", for the user to understand your progress
- Be honest about your limitations and failures, and explain how you are trying to overcome them
- Never call this tool repeatedly without calling other tools - the system will detect repeated standalone calls and enforce finalization
