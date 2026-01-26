const SUPPORT_METADATA_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: [
    'user_language',
    'what_the_user_needs_from_netdata_in_english',
    'categories',
    'search_terms',
    'stopped_before_completion',
    'response_accuracy',
    'response_completeness',
    'user_frustration',
    'netdata_completeness',
    'documentation_completeness',
  ],
  properties: {
    user_language: {
      type: 'string',
      description: 'The language the user speaks. If unsure use English.',
    },
    what_the_user_needs_from_netdata_in_english: {
      type: 'string',
      description: "Describe in English what the user needs from Netdata. If the request is off-topic, say 'off-topic'.",
    },
    categories: {
      type: 'array',
      items: {
        type: 'string',
        enum: [
          'Overview and Concepts',
          'Netdata Agent Installation',
          'Netdata Agent Configuration',
          'Netdata Parents',
          'Data Collection',
          'Logs',
          'OTEL',
          'Alerts / Notifications',
          'Dashboards',
          'Netdata Cloud',
          'Billing and Pricing',
          'Security and Access',
          'Exporting',
          'Data Access / API',
          'Troubleshooting / Errors',
          'Off-Topic / Irrelevant to Netdata',
          'Greeting',
          'Cannot Understand Request / Clarifications',
          'Meta-Query / Introspection',
          'Other',
        ],
      },
      description: 'Ordered categories by priority (left-to-right).',
    },
    search_terms: {
      type: 'array',
      items: {
        type: 'string',
      },
      description: "Related search terms and phrases (do not add 'Netdata' to them)",
    },
    stopped_before_completion: {
      type: 'boolean',
      description: 'Set to true if you were stopped before completing the test, set it to false if the task was completed.',
    },
    response_accuracy: {
      type: 'object',
      required: ['score'],
      properties: {
        score: {
          type: 'string',
          pattern: '^[0-9]+%$',
        },
      },
      description: 'How accurately we followed the `instructions_for_netdata_support_engineers_in_english` field of the advisory (0-100%)? If the response answers precisely and exclusively the `instructions_for_netdata_support_engineers_in_english` set it to 100%. If the response includes more, less, or different information, set it <100%.',
    },
    response_completeness: {
      type: 'object',
      required: ['score', 'limitations'],
      properties: {
        score: {
          type: 'string',
          pattern: '^[0-9]+%$',
        },
        limitations: {
          type: 'array',
          items: {
            type: 'string',
          },
          description: 'Provide a list of limitations of your response. What your response should include to be ideal for this request. If no limitation is found, set this to empty array [].',
        },
      },
      description: 'Did you answer the user request in whole (0-100%)? If the response covers all aspects of `instructions_for_netdata_support_engineers_in_english` set 100%. If there are still any details missing set it to <100%.',
    },
    user_frustration: {
      type: 'object',
      required: ['score'],
      properties: {
        score: {
          type: 'string',
          pattern: '^[0-9]+%$',
        },
      },
      description: 'Are there any indications of user frustration? 0% = no, 100% = the user is mad. If everything looks normal set it to 0%. If there are indications of user discomfort, frustration, or stress, set it to >0%, with 100% being a mad user.',
    },
    netdata_completeness: {
      type: 'object',
      required: ['score', 'limitations'],
      properties: {
        score: {
          type: 'string',
          pattern: '^[0-9]+%$',
        },
        limitations: {
          type: 'array',
          items: {
            type: 'string',
          },
          description: 'Provide a list of netdata limitation based on your research. What netdata should do to support this user better Be detailed. If no limitation is found, set this to empty array [].',
        },
      },
      description: 'How complete is Netdata for satisfying the user request (0-100%)? If Netdata can do what `instructions_for_netdata_support_engineers_in_english` asks for, set it to 100%. If Netdata has limitations, set it to <100%.',
    },
    documentation_completeness: {
      type: 'object',
      required: ['score', 'limitations'],
      properties: {
        score: {
          type: 'string',
          pattern: '^[0-9]+%$',
        },
        limitations: {
          type: 'array',
          items: {
            type: 'string',
          },
          description: 'Provide a list of documentation limitation. If no limitation is found, set this to empty array [].',
        },
      },
      description: "How complete is Netdata's documentation (0-100%)? If the response is answered purely based on documentation, set it to 100%. If you have identified missing or contradicting documentation, set it to <100%.",
    },
  },
};

const SYSTEM_PROMPT_INSTRUCTIONS = [
  'You MUST emit a META JSON object for support request classification.',
  'Use the wrapper: <ai-agent-NONCE-META plugin="support-metadata">{...}</ai-agent-NONCE-META>.',
  'The JSON must match the schema exactly and include every required field.',
  'If the user request is off-topic, set what_the_user_needs_from_netdata_in_english to "off-topic" and include category "Off-Topic / Irrelevant to Netdata".',
  'Categories must be ordered by priority. search_terms must not include the word "Netdata".',
].join('\n');

const XML_NEXT_SNIPPET = 'META support-metadata required: output <ai-agent-NONCE-META plugin="support-metadata">{...}</ai-agent-NONCE-META> with valid JSON matching the schema.';

const FINAL_REPORT_EXAMPLE_SNIPPET = '<ai-agent-NONCE-META plugin="support-metadata">{"user_language":"English","what_the_user_needs_from_netdata_in_english":"How to configure alerts","categories":["Alerts / Notifications"],"search_terms":["alert configuration","health alarm"],"stopped_before_completion":false,"response_accuracy":{"score":"100%"},"response_completeness":{"score":"100%","limitations":[]},"user_frustration":{"score":"0%"},"netdata_completeness":{"score":"100%","limitations":[]},"documentation_completeness":{"score":"100%","limitations":[]}}</ai-agent-NONCE-META>';

// eslint-disable-next-line import/no-default-export -- Final-report plugins require a default export.
export default function supportMetadataPluginFactory() {
  return {
    name: 'support-metadata',
    getRequirements() {
      return {
        schema: SUPPORT_METADATA_SCHEMA,
        systemPromptInstructions: SYSTEM_PROMPT_INSTRUCTIONS,
        xmlNextSnippet: XML_NEXT_SNIPPET,
        finalReportExampleSnippet: FINAL_REPORT_EXAMPLE_SNIPPET,
      };
    },
    async onComplete() {
      // Intentionally no-op; metadata is consumed by downstream systems.
    },
  };
}
