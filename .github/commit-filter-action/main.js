const core = require('@actions/core')
const github = require('@actions/github')

try {
    const regex = core.getInput('regex')
    const invertMatch = core.getInput('invert-match')
    const caseInsensitive = core.getInput('case-insensitive')
    const context = github.context

    const r = new RegExp(regex, `u${caseInsensitive ? 'i' : ''}`)

    const commitMessage = context.event.commits.slice(-1)[0].message

    core.setOutput('matched', (r.test(commitMessage) ? !invertMatch : invertMatch))
} catch (error) {
    core.setFailed(error.message)
}
