/**
 * Debug helper to analyze message structure issues
 */

function debugMessages(messages, title = 'Messages') {
    console.group(`🔍 ${title}`);
    messages.forEach((msg, index) => {
        console.log(`Message ${index}:`, {
            role: msg.role,
            contentType: typeof msg.content,
            contentIsArray: Array.isArray(msg.content),
            content: msg.content
        });
        
        if (Array.isArray(msg.content)) {
            msg.content.forEach((block, blockIndex) => {
                console.log(`  Block ${blockIndex}:`, {
                    type: block.type,
                    hasText: !!block.text,
                    textType: typeof block.text,
                    textIsArray: Array.isArray(block.text)
                });
            });
        }
    });
    console.groupEnd();
}

window.debugMessages = debugMessages;
