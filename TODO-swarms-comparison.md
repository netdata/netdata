# TODO – ai-agent vs swarms comparison

## 1. TL;DR
- Costa asked for an evidence-backed comparison between this repo (ai-agent) and the public `kyegomez/swarms` framework.
- Deliverable must highlight architecture, runtime capabilities, tooling, and trade-offs (features, maturity, DX, ops) with explicit sources.

## 2. Analysis
- **ai-agent facts reviewed**: README.md, docs/SPECS.md, docs/IMPLEMENTATION.md, docs/DESIGN.md, docs/MULTI-AGENT.md, docs/AI-AGENT-GUIDE.md, docs/TESTING.md, docs/AI-AGENT-INTERNAL-API.md (plus related ADRs if needed) describe a TypeScript CLI/library that: loads all config at startup, composes `.ai` prompt files with YAML frontmatter, uses Vercel AI SDK + MCP TypeScript SDK, enforces recursive agent autonomy, provides headends (REST, MCP transports, OpenAI/Anthropic-compatible), telemetry, context guard, accounting, deterministic test harnesses, etc.
- **swarms facts reviewed**: Latest README on GitHub (master branch) outlines a Python multi-agent orchestration toolkit featuring sequential/concurrent workflows, graph orchestration, HeavySwarm, Agent Orchestration Protocol (AOP) server, SwarmRouter, enterprise claims (HA infra, modular microservices, load balancing), CLI/SDK, docs at docs.swarms.world.
- **Overlap/contrast themes identified so far**: (a) language/runtime (TypeScript CLI vs Python library/services), (b) configuration style (frontmatter vs Python constructors), (c) tool integration mechanisms (MCP-first vs custom/multi memory + compatibility with LangChain/AutoGen/CrewAI), (d) orchestration patterns (ai-agent uses recursive LLM-driven planning with agents-as-tools; swarms exposes discrete workflow classes plus router), (e) operational posture (ai-agent emphasises deterministic harness tests, telemetry, OS integration; swarms emphasises HA, microservices, marketplace).

## 3. Decisions Required From Costa
1. **Comparison format** – Do you want the final deliverable as a written brief, table, slide-style bullets, or something else?
2. **Depth vs breadth** – Should we go deep on runtime architecture (config resolution, scheduling, MCP stack) or broaden into ecosystem (community, docs, roadmap)?
3. **Evaluation criteria weighting** – Are you prioritizing developer experience, operational maturity, extensibility, performance, or licensing/governance?
4. **Evidence scope** – Is the GitHub README + docs enough, or should we include blog posts, benchmarks, or user testimonials?

## 4. Plan
1. Finish extracting structured facts from ai-agent docs (arch, capabilities, ops, DX) to anchor comparison.
2. Gather latest swarms documentation/readme snippets + (if available) docs.swarms.world sections describing architecture, workflows, tooling, enterprise claims.
3. Map both products across mutually agreed criteria (architecture, orchestration model, extensibility, DX, ops, maturity, risks) noting facts vs speculation.
4. Produce the comparison report with bullets, evidence citations, explicit risks, and recommended next steps; highlight any missing decisions.

## 5. Implied Decisions
- If additional sources (blog posts, changelogs) are needed, we must confirm they are acceptable references.
- Any quantitative comparison (performance/cost) requires instrumentation data that we currently lack; would need future benchmarking tasks.

## 6. Testing Requirements
- No code change is planned, but if we add runnable examples or scripts later, we must run `npm run lint` and `npm run build` to comply with repo policy.

## 7. Documentation Updates Required
- Comparison results likely remain external (response to Costa). No repo doc updates unless Costa later asks to incorporate findings into README/docs.
