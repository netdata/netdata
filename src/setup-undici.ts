import { Agent, setGlobalDispatcher, type Dispatcher } from 'undici';

const agentOptions: Agent.Options = {
  headersTimeout: 0,
  bodyTimeout: 0,
};

const dispatcher: Dispatcher = new Agent(agentOptions);

setGlobalDispatcher(dispatcher);
