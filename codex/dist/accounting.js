import * as fs from 'fs';
import * as path from 'path';
export class AccountingManager {
    accountingFile;
    callbacks;
    constructor(accountingFile, callbacks) {
        this.accountingFile = accountingFile;
        this.callbacks = callbacks;
    }
    async logEntry(entry) {
        if (this.callbacks?.onAccounting) {
            this.callbacks.onAccounting(entry);
            return;
        }
        if (!this.accountingFile)
            return;
        try {
            const dir = path.dirname(this.accountingFile);
            if (!fs.existsSync(dir))
                fs.mkdirSync(dir, { recursive: true });
            fs.appendFileSync(this.accountingFile, JSON.stringify(entry) + '\n', 'utf-8');
        }
        catch (e) {
            console.error(`Failed to write accounting entry to ${this.accountingFile}:`, e);
        }
    }
    async logLLMUsage(provider, model, tokens, latency, status, error) {
        const entry = { type: 'llm', timestamp: Date.now(), status, latency, provider, model, tokens, error };
        await this.logEntry(entry);
    }
    async logToolUsage(mcpServer, command, charactersIn, charactersOut, latency, status, error) {
        const entry = { type: 'tool', timestamp: Date.now(), status, latency, mcpServer, command, charactersIn, charactersOut, error };
        await this.logEntry(entry);
    }
}
