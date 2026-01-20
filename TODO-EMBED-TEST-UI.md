# Embed Test UI

## TL;DR
Create a test UI for the embed headend with two modes:
1. **Div mode** - Chat that takes over a container div, resizes naturally
2. **Widget mode** - Intercom-style floating chat widget (bottom-right)

## Requirements

### Shared Features
- Dark/light theme: auto-detect system preference + manual toggle
- Markdown rendering: markdown-it (CDN)
- Mermaid charts: mermaid.js (CDN)
- Copy functionality:
  - Copy individual LLM response (as markdown)
  - Copy entire conversation
  - Copy code blocks
- Real-time streaming updates
- Spinner with last status update (vanishes when output starts streaming)
- Conversation history persisted in localStorage
- Clear button to reset conversation
- Auto-expanding textarea input (grows with content)
- HTML-to-markdown conversion when pasting
- Bottom status bar: small, single line (status | clear | theme toggle)
- Hardcoded agent: `support.ai`

### Div Mode (`test-div.html`)
- Takes over a container div
- Resizes naturally with the div

### Widget Mode (`test-widget.html`)
- Bottom-right floating chat bubble icon (customizable)
- Click to expand/collapse chat
- Maximize button
- Survives page navigation (same origin)

## File Structure
```
src/headends/embed-test/
├── test-div.html      # Div mode demo page
├── test-widget.html   # Widget mode demo page
├── test.css           # Shared styles
└── test.js            # Shared JavaScript
```

## Decisions

### User Decisions Required
None - all requirements are clear.

### Implied Decisions
1. Serve files from embed headend at `/test-div.html`, `/test-widget.html`, `/test.css`, `/test.js`
2. Use CDN for markdown-it and mermaid (not bundled)
3. Widget icon: chat bubble SVG (user can override via config)
4. Enter to send, Shift+Enter for newline in textarea
5. Maximize in widget mode = near full-screen with padding

## Implementation Plan

### Phase 1: Core JavaScript (`test.js`)
1. Create `AiAgentChatUI` class that:
   - Initializes inside a container element
   - Manages conversation state
   - Handles localStorage persistence
   - Renders messages with markdown/mermaid
   - Manages theme switching
   - Provides copy functionality
   - Handles streaming responses

### Phase 2: Shared Styles (`test.css`)
1. CSS variables for theming (light/dark)
2. Message bubbles (user/assistant)
3. Auto-expanding textarea
4. Spinner animation
5. Status bar styling
6. Code block styling with copy button
7. Responsive layout

### Phase 3: Div Mode (`test-div.html`)
1. Simple page with a resizable container div
2. Initialize `AiAgentChatUI` in div mode

### Phase 4: Widget Mode (`test-widget.html`)
1. Floating button component
2. Expandable chat panel
3. Maximize/minimize functionality
4. Initialize `AiAgentChatUI` in widget mode

### Phase 5: Server Integration
1. Update `embed-headend.ts` to serve test files
2. Add routes for `/test-div.html`, `/test-widget.html`, `/test.css`, `/test.js`

## Testing Requirements
- Manual testing with running embed headend
- Test both modes in light/dark themes
- Test streaming, copy, clear functionality
- Test localStorage persistence
- Test mermaid chart rendering

## Documentation Updates
- Update `docs/Headends-Embed.md` to mention test UI


## ISSUES (added by Costa)

- The spinner is removed when the model sends conents, but it is not added back when the model starting sending reasoning tags or any other tags.
  This is a problem because the model frequently says:
  > I'll research X. Let me gather specifics...
  and immediately continues working. So, output from the model, is not necessarily the end of the spinner.
  The spinner should stop while the model is sending output, but it should continue when the model sends progress, reasoning, etc.

- At the lower left corner there is a status line. This stays at "Connecting..." forever

- markdown parser:
  - allow html in markdown
  - noticed that it turns `(C)` into the copyright symbol, but this is problematic for devops, because `(C)` means a program written in C. Can it be disabled?

- the solution must set the format to `markdown+mermaid` so that the model knows it can use mermaid charts instead of text blocks art

- text input:
  - must preserve newlines
  - it converts html to markdown on paste, but it must render markdown at the user bubble

- agent response:
  - the left-right padding must be dynamic depending on the width of the underlying parent div. The wider the screen the more padding should exist.
  - code blocks should have line spacing 1.0 so that ascii art is shown correctly (no dotted lines)
  - monospace font is significantly smaller than the normal one - code blocks are prime content for configurations, we need them to match the page font


