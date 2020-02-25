# plugin-rest-endpoint-methods.js

> Octokit plugin adding one method for all of api.github.com REST API endpoints

[![@latest](https://img.shields.io/npm/v/@octokit/plugin-rest-endpoint-methods.svg)](https://www.npmjs.com/package/@octokit/plugin-rest-endpoint-methods)
[![Build Status](https://github.com/octokit/plugin-rest-endpoint-methods.js/workflows/Test/badge.svg)](https://github.com/octokit/plugin-rest-endpoint-methods.js/actions?workflow=Test)
[![Greenkeeper](https://badges.greenkeeper.io/octokit/plugin-rest-endpoint-methods.js.svg)](https://greenkeeper.io/)

## Usage

<table>
<tbody valign=top align=left>
<tr><th>
Browsers
</th><td width=100%>

Load `@octokit/plugin-rest-endpoint-methods` and [`@octokit/core`](https://github.com/octokit/core.js) (or core-compatible module) directly from [cdn.pika.dev](https://cdn.pika.dev)

```html
<script type="module">
  import { Octokit } from "https://cdn.pika.dev/@octokit/core";
  import { restEndpointMethods } from "https://cdn.pika.dev/@octokit/plugin-rest-endpoint-methods";
</script>
```

</td></tr>
<tr><th>
Node
</th><td>

Install with `npm install @octokit/core @octokit/plugin-rest-endpoint-methods`. Optionally replace `@octokit/core` with a compatible module

```js
const { Octokit } = require("@octokit/core");
const {
  restEndpointMethods
} = require("@octokit/plugin-rest-endpoint-methods");
```

</td></tr>
</tbody>
</table>

```js
const MyOctokit = Octokit.plugin(restEndpointMethods);
const octokit = new MyOctokit({ auth: "secret123" });

// https://developer.github.com/v3/apps/#get-the-authenticated-github-app
octokit.apps.getAuthenticated();

// https://developer.github.com/v3/apps/#create-a-github-app-from-a-manifest
octokit.apps.createFromManifest({ code });

// https://developer.github.com/v3/apps/#list-installations
octokit.apps.listInstallations();

// https://developer.github.com/v3/apps/#get-an-installation
octokit.apps.getInstallation({ installation_id });

// https://developer.github.com/v3/apps/#delete-an-installation
octokit.apps.deleteInstallation({ installation_id });

// https://developer.github.com/v3/apps/#create-a-new-installation-token
octokit.apps.createInstallationToken({
  installation_id,
  repository_ids,
  permissions
});

// https://developer.github.com/v3/oauth_authorizations/#list-your-grants
octokit.oauthAuthorizations.listGrants();

// https://developer.github.com/v3/oauth_authorizations/#get-a-single-grant
octokit.oauthAuthorizations.getGrant({ grant_id });

// https://developer.github.com/v3/oauth_authorizations/#delete-a-grant
octokit.oauthAuthorizations.deleteGrant({ grant_id });

// https://developer.github.com/v3/apps/oauth_applications/#delete-an-app-authorization
octokit.apps.deleteAuthorization({ client_id, access_token });

// https://developer.github.com/v3/apps/oauth_applications/#revoke-a-grant-for-an-application
octokit.apps.revokeGrantForApplication({ client_id, access_token });

// DEPRECATED: octokit.oauthAuthorizations.revokeGrantForApplication() has been renamed to octokit.apps.revokeGrantForApplication()
octokit.oauthAuthorizations.revokeGrantForApplication({
  client_id,
  access_token
});

// https://developer.github.com/v3/apps/oauth_applications/#check-a-token
octokit.apps.checkToken({ client_id, access_token });

// https://developer.github.com/v3/apps/oauth_applications/#reset-a-token
octokit.apps.resetToken({ client_id, access_token });

// https://developer.github.com/v3/apps/oauth_applications/#delete-an-app-token
octokit.apps.deleteToken({ client_id, access_token });

// https://developer.github.com/v3/apps/oauth_applications/#check-an-authorization
octokit.apps.checkAuthorization({ client_id, access_token });

// DEPRECATED: octokit.oauthAuthorizations.checkAuthorization() has been renamed to octokit.apps.checkAuthorization()
octokit.oauthAuthorizations.checkAuthorization({ client_id, access_token });

// https://developer.github.com/v3/apps/oauth_applications/#reset-an-authorization
octokit.apps.resetAuthorization({ client_id, access_token });

// DEPRECATED: octokit.oauthAuthorizations.resetAuthorization() has been renamed to octokit.apps.resetAuthorization()
octokit.oauthAuthorizations.resetAuthorization({ client_id, access_token });

// https://developer.github.com/v3/apps/oauth_applications/#revoke-an-authorization-for-an-application
octokit.apps.revokeAuthorizationForApplication({ client_id, access_token });

// DEPRECATED: octokit.oauthAuthorizations.revokeAuthorizationForApplication() has been renamed to octokit.apps.revokeAuthorizationForApplication()
octokit.oauthAuthorizations.revokeAuthorizationForApplication({
  client_id,
  access_token
});

// https://developer.github.com/v3/apps/#get-a-single-github-app
octokit.apps.getBySlug({ app_slug });

// https://developer.github.com/v3/oauth_authorizations/#list-your-authorizations
octokit.oauthAuthorizations.listAuthorizations();

// https://developer.github.com/v3/oauth_authorizations/#create-a-new-authorization
octokit.oauthAuthorizations.createAuthorization({
  scopes,
  note,
  note_url,
  client_id,
  client_secret,
  fingerprint
});

// https://developer.github.com/v3/oauth_authorizations/#get-or-create-an-authorization-for-a-specific-app
octokit.oauthAuthorizations.getOrCreateAuthorizationForApp({
  client_id,
  client_secret,
  scopes,
  note,
  note_url,
  fingerprint
});

// https://developer.github.com/v3/oauth_authorizations/#get-or-create-an-authorization-for-a-specific-app-and-fingerprint
octokit.oauthAuthorizations.getOrCreateAuthorizationForAppAndFingerprint({
  client_id,
  fingerprint,
  client_secret,
  scopes,
  note,
  note_url
});

// DEPRECATED: octokit.oauthAuthorizations.getOrCreateAuthorizationForAppFingerprint() has been renamed to octokit.oauthAuthorizations.getOrCreateAuthorizationForAppAndFingerprint()
octokit.oauthAuthorizations.getOrCreateAuthorizationForAppFingerprint({
  client_id,
  fingerprint,
  client_secret,
  scopes,
  note,
  note_url
});

// https://developer.github.com/v3/oauth_authorizations/#get-a-single-authorization
octokit.oauthAuthorizations.getAuthorization({ authorization_id });

// https://developer.github.com/v3/oauth_authorizations/#update-an-existing-authorization
octokit.oauthAuthorizations.updateAuthorization({
  authorization_id,
  scopes,
  add_scopes,
  remove_scopes,
  note,
  note_url,
  fingerprint
});

// https://developer.github.com/v3/oauth_authorizations/#delete-an-authorization
octokit.oauthAuthorizations.deleteAuthorization({ authorization_id });

// https://developer.github.com/v3/codes_of_conduct/#list-all-codes-of-conduct
octokit.codesOfConduct.listConductCodes();

// https://developer.github.com/v3/codes_of_conduct/#get-an-individual-code-of-conduct
octokit.codesOfConduct.getConductCode({ key });

// https://developer.github.com/v3/apps/installations/#create-a-content-attachment
octokit.apps.createContentAttachment({ content_reference_id, title, body });

// https://developer.github.com/v3/emojis/#emojis
octokit.emojis.get();

// https://developer.github.com/v3/activity/events/#list-public-events
octokit.activity.listPublicEvents();

// https://developer.github.com/v3/activity/feeds/#list-feeds
octokit.activity.listFeeds();

// https://developer.github.com/v3/gists/#list-a-users-gists
octokit.gists.list({ since });

// https://developer.github.com/v3/gists/#create-a-gist
octokit.gists.create({ files, description, public });

// https://developer.github.com/v3/gists/#list-all-public-gists
octokit.gists.listPublic({ since });

// https://developer.github.com/v3/gists/#list-starred-gists
octokit.gists.listStarred({ since });

// https://developer.github.com/v3/gists/#get-a-single-gist
octokit.gists.get({ gist_id });

// https://developer.github.com/v3/gists/#edit-a-gist
octokit.gists.update({ gist_id, description, files });

// https://developer.github.com/v3/gists/#delete-a-gist
octokit.gists.delete({ gist_id });

// https://developer.github.com/v3/gists/comments/#list-comments-on-a-gist
octokit.gists.listComments({ gist_id });

// https://developer.github.com/v3/gists/comments/#create-a-comment
octokit.gists.createComment({ gist_id, body });

// https://developer.github.com/v3/gists/comments/#get-a-single-comment
octokit.gists.getComment({ gist_id, comment_id });

// https://developer.github.com/v3/gists/comments/#edit-a-comment
octokit.gists.updateComment({ gist_id, comment_id, body });

// https://developer.github.com/v3/gists/comments/#delete-a-comment
octokit.gists.deleteComment({ gist_id, comment_id });

// https://developer.github.com/v3/gists/#list-gist-commits
octokit.gists.listCommits({ gist_id });

// https://developer.github.com/v3/gists/#fork-a-gist
octokit.gists.fork({ gist_id });

// https://developer.github.com/v3/gists/#list-gist-forks
octokit.gists.listForks({ gist_id });

// https://developer.github.com/v3/gists/#star-a-gist
octokit.gists.star({ gist_id });

// https://developer.github.com/v3/gists/#unstar-a-gist
octokit.gists.unstar({ gist_id });

// https://developer.github.com/v3/gists/#check-if-a-gist-is-starred
octokit.gists.checkIsStarred({ gist_id });

// https://developer.github.com/v3/gists/#get-a-specific-revision-of-a-gist
octokit.gists.getRevision({ gist_id, sha });

// https://developer.github.com/v3/gitignore/#listing-available-templates
octokit.gitignore.listTemplates();

// https://developer.github.com/v3/gitignore/#get-a-single-template
octokit.gitignore.getTemplate({ name });

// https://developer.github.com/v3/apps/installations/#list-repositories
octokit.apps.listRepos();

// https://developer.github.com/v3/apps/installations/#revoke-an-installation-token
octokit.apps.revokeInstallationToken();

// https://developer.github.com/v3/issues/#list-issues
octokit.issues.list({ filter, state, labels, sort, direction, since });

// https://developer.github.com/v3/licenses/#list-commonly-used-licenses
octokit.licenses.listCommonlyUsed();

// DEPRECATED: octokit.licenses.list() has been renamed to octokit.licenses.listCommonlyUsed()
octokit.licenses.list();

// https://developer.github.com/v3/licenses/#get-an-individual-license
octokit.licenses.get({ license });

// https://developer.github.com/v3/markdown/#render-an-arbitrary-markdown-document
octokit.markdown.render({ text, mode, context });

// https://developer.github.com/v3/markdown/#render-a-markdown-document-in-raw-mode
octokit.markdown.renderRaw({ data });

// https://developer.github.com/v3/apps/marketplace/#check-if-a-github-account-is-associated-with-any-marketplace-listing
octokit.apps.checkAccountIsAssociatedWithAny({ account_id });

// https://developer.github.com/v3/apps/marketplace/#list-all-plans-for-your-marketplace-listing
octokit.apps.listPlans();

// https://developer.github.com/v3/apps/marketplace/#list-all-github-accounts-user-or-organization-on-a-specific-plan
octokit.apps.listAccountsUserOrOrgOnPlan({ plan_id, sort, direction });

// https://developer.github.com/v3/apps/marketplace/#check-if-a-github-account-is-associated-with-any-marketplace-listing
octokit.apps.checkAccountIsAssociatedWithAnyStubbed({ account_id });

// https://developer.github.com/v3/apps/marketplace/#list-all-plans-for-your-marketplace-listing
octokit.apps.listPlansStubbed();

// https://developer.github.com/v3/apps/marketplace/#list-all-github-accounts-user-or-organization-on-a-specific-plan
octokit.apps.listAccountsUserOrOrgOnPlanStubbed({ plan_id, sort, direction });

// https://developer.github.com/v3/meta/#meta
octokit.meta.get();

// https://developer.github.com/v3/activity/events/#list-public-events-for-a-network-of-repositories
octokit.activity.listPublicEventsForRepoNetwork({ owner, repo });

// https://developer.github.com/v3/activity/notifications/#list-your-notifications
octokit.activity.listNotifications({ all, participating, since, before });

// https://developer.github.com/v3/activity/notifications/#mark-as-read
octokit.activity.markAsRead({ last_read_at });

// https://developer.github.com/v3/activity/notifications/#view-a-single-thread
octokit.activity.getThread({ thread_id });

// https://developer.github.com/v3/activity/notifications/#mark-a-thread-as-read
octokit.activity.markThreadAsRead({ thread_id });

// https://developer.github.com/v3/activity/notifications/#get-a-thread-subscription
octokit.activity.getThreadSubscription({ thread_id });

// https://developer.github.com/v3/activity/notifications/#set-a-thread-subscription
octokit.activity.setThreadSubscription({ thread_id, ignored });

// https://developer.github.com/v3/activity/notifications/#delete-a-thread-subscription
octokit.activity.deleteThreadSubscription({ thread_id });

// https://developer.github.com/v3/orgs/#list-all-organizations
octokit.orgs.list({ since });

// https://developer.github.com/v3/orgs/#get-an-organization
octokit.orgs.get({ org });

// https://developer.github.com/v3/orgs/#edit-an-organization
octokit.orgs.update({
  org,
  billing_email,
  company,
  email,
  location,
  name,
  description,
  has_organization_projects,
  has_repository_projects,
  default_repository_permission,
  members_can_create_repositories,
  members_can_create_internal_repositories,
  members_can_create_private_repositories,
  members_can_create_public_repositories,
  members_allowed_repository_creation_type
});

// https://developer.github.com/v3/orgs/blocking/#list-blocked-users
octokit.orgs.listBlockedUsers({ org });

// https://developer.github.com/v3/orgs/blocking/#check-whether-a-user-is-blocked-from-an-organization
octokit.orgs.checkBlockedUser({ org, username });

// https://developer.github.com/v3/orgs/blocking/#block-a-user
octokit.orgs.blockUser({ org, username });

// https://developer.github.com/v3/orgs/blocking/#unblock-a-user
octokit.orgs.unblockUser({ org, username });

// https://developer.github.com/v3/activity/events/#list-public-events-for-an-organization
octokit.activity.listPublicEventsForOrg({ org });

// https://developer.github.com/v3/orgs/hooks/#list-hooks
octokit.orgs.listHooks({ org });

// https://developer.github.com/v3/orgs/hooks/#create-a-hook
octokit.orgs.createHook({ org, name, config, events, active });

// https://developer.github.com/v3/orgs/hooks/#get-single-hook
octokit.orgs.getHook({ org, hook_id });

// https://developer.github.com/v3/orgs/hooks/#edit-a-hook
octokit.orgs.updateHook({ org, hook_id, config, events, active });

// https://developer.github.com/v3/orgs/hooks/#delete-a-hook
octokit.orgs.deleteHook({ org, hook_id });

// https://developer.github.com/v3/orgs/hooks/#ping-a-hook
octokit.orgs.pingHook({ org, hook_id });

// https://developer.github.com/v3/apps/#get-an-organization-installation
octokit.apps.getOrgInstallation({ org });

// DEPRECATED: octokit.apps.findOrgInstallation() has been renamed to octokit.apps.getOrgInstallation()
octokit.apps.findOrgInstallation({ org });

// https://developer.github.com/v3/orgs/#list-installations-for-an-organization
octokit.orgs.listInstallations({ org });

// https://developer.github.com/v3/interactions/orgs/#get-interaction-restrictions-for-an-organization
octokit.interactions.getRestrictionsForOrg({ org });

// https://developer.github.com/v3/interactions/orgs/#add-or-update-interaction-restrictions-for-an-organization
octokit.interactions.addOrUpdateRestrictionsForOrg({ org, limit });

// https://developer.github.com/v3/interactions/orgs/#remove-interaction-restrictions-for-an-organization
octokit.interactions.removeRestrictionsForOrg({ org });

// https://developer.github.com/v3/orgs/members/#list-pending-organization-invitations
octokit.orgs.listPendingInvitations({ org });

// https://developer.github.com/v3/orgs/members/#create-organization-invitation
octokit.orgs.createInvitation({ org, invitee_id, email, role, team_ids });

// https://developer.github.com/v3/orgs/members/#list-organization-invitation-teams
octokit.orgs.listInvitationTeams({ org, invitation_id });

// https://developer.github.com/v3/issues/#list-issues
octokit.issues.listForOrg({
  org,
  filter,
  state,
  labels,
  sort,
  direction,
  since
});

// https://developer.github.com/v3/orgs/members/#members-list
octokit.orgs.listMembers({ org, filter, role });

// https://developer.github.com/v3/orgs/members/#check-membership
octokit.orgs.checkMembership({ org, username });

// https://developer.github.com/v3/orgs/members/#remove-a-member
octokit.orgs.removeMember({ org, username });

// https://developer.github.com/v3/orgs/members/#get-organization-membership
octokit.orgs.getMembership({ org, username });

// https://developer.github.com/v3/orgs/members/#add-or-update-organization-membership
octokit.orgs.addOrUpdateMembership({ org, username, role });

// https://developer.github.com/v3/orgs/members/#remove-organization-membership
octokit.orgs.removeMembership({ org, username });

// https://developer.github.com/v3/migrations/orgs/#start-an-organization-migration
octokit.migrations.startForOrg({
  org,
  repositories,
  lock_repositories,
  exclude_attachments
});

// https://developer.github.com/v3/migrations/orgs/#list-organization-migrations
octokit.migrations.listForOrg({ org });

// https://developer.github.com/v3/migrations/orgs/#get-the-status-of-an-organization-migration
octokit.migrations.getStatusForOrg({ org, migration_id });

// https://developer.github.com/v3/migrations/orgs/#download-an-organization-migration-archive
octokit.migrations.downloadArchiveForOrg({ org, migration_id });

// DEPRECATED: octokit.migrations.getArchiveForOrg() has been renamed to octokit.migrations.downloadArchiveForOrg()
octokit.migrations.getArchiveForOrg({ org, migration_id });

// https://developer.github.com/v3/migrations/orgs/#delete-an-organization-migration-archive
octokit.migrations.deleteArchiveForOrg({ org, migration_id });

// https://developer.github.com/v3/migrations/orgs/#unlock-an-organization-repository
octokit.migrations.unlockRepoForOrg({ org, migration_id, repo_name });

// https://developer.github.com/v3/migrations/orgs/#list-repositories-in-an-organization-migration
octokit.migrations.listReposForOrg({ org, migration_id });

// https://developer.github.com/v3/orgs/outside_collaborators/#list-outside-collaborators
octokit.orgs.listOutsideCollaborators({ org, filter });

// https://developer.github.com/v3/orgs/outside_collaborators/#remove-outside-collaborator
octokit.orgs.removeOutsideCollaborator({ org, username });

// https://developer.github.com/v3/orgs/outside_collaborators/#convert-member-to-outside-collaborator
octokit.orgs.convertMemberToOutsideCollaborator({ org, username });

// https://developer.github.com/v3/projects/#list-organization-projects
octokit.projects.listForOrg({ org, state });

// https://developer.github.com/v3/projects/#create-an-organization-project
octokit.projects.createForOrg({ org, name, body });

// https://developer.github.com/v3/orgs/members/#public-members-list
octokit.orgs.listPublicMembers({ org });

// https://developer.github.com/v3/orgs/members/#check-public-membership
octokit.orgs.checkPublicMembership({ org, username });

// https://developer.github.com/v3/orgs/members/#publicize-a-users-membership
octokit.orgs.publicizeMembership({ org, username });

// https://developer.github.com/v3/orgs/members/#conceal-a-users-membership
octokit.orgs.concealMembership({ org, username });

// https://developer.github.com/v3/repos/#list-organization-repositories
octokit.repos.listForOrg({ org, type, sort, direction });

// https://developer.github.com/v3/repos/#create
octokit.repos.createInOrg({
  org,
  name,
  description,
  homepage,
  private,
  visibility,
  has_issues,
  has_projects,
  has_wiki,
  is_template,
  team_id,
  auto_init,
  gitignore_template,
  license_template,
  allow_squash_merge,
  allow_merge_commit,
  allow_rebase_merge,
  delete_branch_on_merge
});

// https://developer.github.com/v3/teams/#list-teams
octokit.teams.list({ org });

// https://developer.github.com/v3/teams/#create-team
octokit.teams.create({
  org,
  name,
  description,
  maintainers,
  repo_names,
  privacy,
  permission,
  parent_team_id
});

// https://developer.github.com/v3/teams/#get-team-by-name
octokit.teams.getByName({ org, team_slug });

// https://developer.github.com/v3/teams/#edit-team
octokit.teams.updateInOrg({
  org,
  team_slug,
  name,
  description,
  privacy,
  permission,
  parent_team_id
});

// https://developer.github.com/v3/teams/#delete-team
octokit.teams.deleteInOrg({ org, team_slug });

// https://developer.github.com/v3/teams/discussions/#list-discussions
octokit.teams.listDiscussionsInOrg({ org, team_slug, direction });

// https://developer.github.com/v3/teams/discussions/#create-a-discussion
octokit.teams.createDiscussionInOrg({ org, team_slug, title, body, private });

// https://developer.github.com/v3/teams/discussions/#get-a-single-discussion
octokit.teams.getDiscussionInOrg({ org, team_slug, discussion_number });

// https://developer.github.com/v3/teams/discussions/#edit-a-discussion
octokit.teams.updateDiscussionInOrg({
  org,
  team_slug,
  discussion_number,
  title,
  body
});

// https://developer.github.com/v3/teams/discussions/#delete-a-discussion
octokit.teams.deleteDiscussionInOrg({ org, team_slug, discussion_number });

// https://developer.github.com/v3/teams/discussion_comments/#list-comments
octokit.teams.listDiscussionCommentsInOrg({
  org,
  team_slug,
  discussion_number,
  direction
});

// https://developer.github.com/v3/teams/discussion_comments/#create-a-comment
octokit.teams.createDiscussionCommentInOrg({
  org,
  team_slug,
  discussion_number,
  body
});

// https://developer.github.com/v3/teams/discussion_comments/#get-a-single-comment
octokit.teams.getDiscussionCommentInOrg({
  org,
  team_slug,
  discussion_number,
  comment_number
});

// https://developer.github.com/v3/teams/discussion_comments/#edit-a-comment
octokit.teams.updateDiscussionCommentInOrg({
  org,
  team_slug,
  discussion_number,
  comment_number,
  body
});

// https://developer.github.com/v3/teams/discussion_comments/#delete-a-comment
octokit.teams.deleteDiscussionCommentInOrg({
  org,
  team_slug,
  discussion_number,
  comment_number
});

// https://developer.github.com/v3/reactions/#list-reactions-for-a-team-discussion-comment
octokit.reactions.listForTeamDiscussionCommentInOrg({
  org,
  team_slug,
  discussion_number,
  comment_number,
  content
});

// https://developer.github.com/v3/reactions/#create-reaction-for-a-team-discussion-comment
octokit.reactions.createForTeamDiscussionCommentInOrg({
  org,
  team_slug,
  discussion_number,
  comment_number,
  content
});

// https://developer.github.com/v3/reactions/#list-reactions-for-a-team-discussion
octokit.reactions.listForTeamDiscussionInOrg({
  org,
  team_slug,
  discussion_number,
  content
});

// https://developer.github.com/v3/reactions/#create-reaction-for-a-team-discussion
octokit.reactions.createForTeamDiscussionInOrg({
  org,
  team_slug,
  discussion_number,
  content
});

// https://developer.github.com/v3/teams/members/#list-pending-team-invitations
octokit.teams.listPendingInvitationsInOrg({ org, team_slug });

// https://developer.github.com/v3/teams/members/#list-team-members
octokit.teams.listMembersInOrg({ org, team_slug, role });

// https://developer.github.com/v3/teams/members/#get-team-membership
octokit.teams.getMembershipInOrg({ org, team_slug, username });

// https://developer.github.com/v3/teams/members/#add-or-update-team-membership
octokit.teams.addOrUpdateMembershipInOrg({ org, team_slug, username, role });

// https://developer.github.com/v3/teams/members/#remove-team-membership
octokit.teams.removeMembershipInOrg({ org, team_slug, username });

// https://developer.github.com/v3/teams/#list-team-projects
octokit.teams.listProjectsInOrg({ org, team_slug });

// https://developer.github.com/v3/teams/#review-a-team-project
octokit.teams.reviewProjectInOrg({ org, team_slug, project_id });

// https://developer.github.com/v3/teams/#add-or-update-team-project
octokit.teams.addOrUpdateProjectInOrg({
  org,
  team_slug,
  project_id,
  permission
});

// https://developer.github.com/v3/teams/#remove-team-project
octokit.teams.removeProjectInOrg({ org, team_slug, project_id });

// https://developer.github.com/v3/teams/#list-team-repos
octokit.teams.listReposInOrg({ org, team_slug });

// https://developer.github.com/v3/teams/#check-if-a-team-manages-a-repository
octokit.teams.checkManagesRepoInOrg({ org, team_slug, owner, repo });

// https://developer.github.com/v3/teams/#add-or-update-team-repository
octokit.teams.addOrUpdateRepoInOrg({ org, team_slug, owner, repo, permission });

// https://developer.github.com/v3/teams/#remove-team-repository
octokit.teams.removeRepoInOrg({ org, team_slug, owner, repo });

// https://developer.github.com/v3/teams/#list-child-teams
octokit.teams.listChildInOrg({ org, team_slug });

// https://developer.github.com/v3/projects/cards/#get-a-project-card
octokit.projects.getCard({ card_id });

// https://developer.github.com/v3/projects/cards/#update-a-project-card
octokit.projects.updateCard({ card_id, note, archived });

// https://developer.github.com/v3/projects/cards/#delete-a-project-card
octokit.projects.deleteCard({ card_id });

// https://developer.github.com/v3/projects/cards/#move-a-project-card
octokit.projects.moveCard({ card_id, position, column_id });

// https://developer.github.com/v3/projects/columns/#get-a-project-column
octokit.projects.getColumn({ column_id });

// https://developer.github.com/v3/projects/columns/#update-a-project-column
octokit.projects.updateColumn({ column_id, name });

// https://developer.github.com/v3/projects/columns/#delete-a-project-column
octokit.projects.deleteColumn({ column_id });

// https://developer.github.com/v3/projects/cards/#list-project-cards
octokit.projects.listCards({ column_id, archived_state });

// https://developer.github.com/v3/projects/cards/#create-a-project-card
octokit.projects.createCard({ column_id, note, content_id, content_type });

// https://developer.github.com/v3/projects/columns/#move-a-project-column
octokit.projects.moveColumn({ column_id, position });

// https://developer.github.com/v3/projects/#get-a-project
octokit.projects.get({ project_id });

// https://developer.github.com/v3/projects/#update-a-project
octokit.projects.update({
  project_id,
  name,
  body,
  state,
  organization_permission,
  private
});

// https://developer.github.com/v3/projects/#delete-a-project
octokit.projects.delete({ project_id });

// https://developer.github.com/v3/projects/collaborators/#list-collaborators
octokit.projects.listCollaborators({ project_id, affiliation });

// https://developer.github.com/v3/projects/collaborators/#add-user-as-a-collaborator
octokit.projects.addCollaborator({ project_id, username, permission });

// https://developer.github.com/v3/projects/collaborators/#remove-user-as-a-collaborator
octokit.projects.removeCollaborator({ project_id, username });

// https://developer.github.com/v3/projects/collaborators/#review-a-users-permission-level
octokit.projects.reviewUserPermissionLevel({ project_id, username });

// https://developer.github.com/v3/projects/columns/#list-project-columns
octokit.projects.listColumns({ project_id });

// https://developer.github.com/v3/projects/columns/#create-a-project-column
octokit.projects.createColumn({ project_id, name });

// https://developer.github.com/v3/rate_limit/#get-your-current-rate-limit-status
octokit.rateLimit.get();

// https://developer.github.com/v3/reactions/#delete-a-reaction
octokit.reactions.delete({ reaction_id });

// https://developer.github.com/v3/repos/#get
octokit.repos.get({ owner, repo });

// https://developer.github.com/v3/repos/#edit
octokit.repos.update({
  owner,
  repo,
  name,
  description,
  homepage,
  private,
  visibility,
  has_issues,
  has_projects,
  has_wiki,
  is_template,
  default_branch,
  allow_squash_merge,
  allow_merge_commit,
  allow_rebase_merge,
  delete_branch_on_merge,
  archived
});

// https://developer.github.com/v3/repos/#delete-a-repository
octokit.repos.delete({ owner, repo });

// https://developer.github.com/v3/actions/artifacts/#get-an-artifact
octokit.actions.getArtifact({ owner, repo, artifact_id });

// https://developer.github.com/v3/actions/artifacts/#delete-an-artifact
octokit.actions.deleteArtifact({ owner, repo, artifact_id });

// https://developer.github.com/v3/actions/artifacts/#download-an-artifact
octokit.actions.downloadArtifact({ owner, repo, artifact_id, archive_format });

// https://developer.github.com/v3/actions/workflow_jobs/#get-a-workflow-job
octokit.actions.getWorkflowJob({ owner, repo, job_id });

// https://developer.github.com/v3/actions/workflow_jobs/#list-workflow-job-logs
octokit.actions.listWorkflowJobLogs({ owner, repo, job_id });

// https://developer.github.com/v3/actions/self_hosted_runners/#list-self-hosted-runners-for-a-repository
octokit.actions.listSelfHostedRunnersForRepo({ owner, repo });

// https://developer.github.com/v3/actions/self_hosted_runners/#list-downloads-for-the-self-hosted-runner-application
octokit.actions.listDownloadsForSelfHostedRunnerApplication({ owner, repo });

// https://developer.github.com/v3/actions/self_hosted_runners/#create-a-registration-token
octokit.actions.createRegistrationToken({ owner, repo });

// https://developer.github.com/v3/actions/self_hosted_runners/#create-a-remove-token
octokit.actions.createRemoveToken({ owner, repo });

// https://developer.github.com/v3/actions/self_hosted_runners/#get-a-self-hosted-runner
octokit.actions.getSelfHostedRunner({ owner, repo, runner_id });

// https://developer.github.com/v3/actions/self_hosted_runners/#remove-a-self-hosted-runner
octokit.actions.removeSelfHostedRunner({ owner, repo, runner_id });

// https://developer.github.com/v3/actions/workflow_runs/#list-repository-workflow-runs
octokit.actions.listRepoWorkflowRuns({
  owner,
  repo,
  actor,
  branch,
  event,
  status
});

// https://developer.github.com/v3/actions/workflow_runs/#get-a-workflow-run
octokit.actions.getWorkflowRun({ owner, repo, run_id });

// https://developer.github.com/v3/actions/artifacts/#list-workflow-run-artifacts
octokit.actions.listWorkflowRunArtifacts({ owner, repo, run_id });

// https://developer.github.com/v3/actions/workflow_runs/#cancel-a-workflow-run
octokit.actions.cancelWorkflowRun({ owner, repo, run_id });

// https://developer.github.com/v3/actions/workflow_jobs/#list-jobs-for-a-workflow-run
octokit.actions.listJobsForWorkflowRun({ owner, repo, run_id });

// https://developer.github.com/v3/actions/workflow_runs/#list-workflow-run-logs
octokit.actions.listWorkflowRunLogs({ owner, repo, run_id });

// https://developer.github.com/v3/actions/workflow_runs/#re-run-a-workflow
octokit.actions.reRunWorkflow({ owner, repo, run_id });

// https://developer.github.com/v3/actions/secrets/#list-secrets-for-a-repository
octokit.actions.listSecretsForRepo({ owner, repo });

// https://developer.github.com/v3/actions/secrets/#get-your-public-key
octokit.actions.getPublicKey({ owner, repo });

// https://developer.github.com/v3/actions/secrets/#get-a-secret
octokit.actions.getSecret({ owner, repo, name });

// https://developer.github.com/v3/actions/secrets/#create-or-update-a-secret-for-a-repository
octokit.actions.createOrUpdateSecretForRepo({
  owner,
  repo,
  name,
  encrypted_value,
  key_id
});

// https://developer.github.com/v3/actions/secrets/#delete-a-secret-from-a-repository
octokit.actions.deleteSecretFromRepo({ owner, repo, name });

// https://developer.github.com/v3/actions/workflows/#list-repository-workflows
octokit.actions.listRepoWorkflows({ owner, repo });

// https://developer.github.com/v3/actions/workflows/#get-a-workflow
octokit.actions.getWorkflow({ owner, repo, workflow_id });

// https://developer.github.com/v3/actions/workflow_runs/#list-workflow-runs
octokit.actions.listWorkflowRuns({
  owner,
  repo,
  workflow_id,
  actor,
  branch,
  event,
  status
});

// https://developer.github.com/v3/issues/assignees/#list-assignees
octokit.issues.listAssignees({ owner, repo });

// https://developer.github.com/v3/issues/assignees/#check-assignee
octokit.issues.checkAssignee({ owner, repo, assignee });

// https://developer.github.com/v3/repos/#enable-automated-security-fixes
octokit.repos.enableAutomatedSecurityFixes({ owner, repo });

// https://developer.github.com/v3/repos/#disable-automated-security-fixes
octokit.repos.disableAutomatedSecurityFixes({ owner, repo });

// https://developer.github.com/v3/repos/branches/#list-branches
octokit.repos.listBranches({ owner, repo, protected });

// https://developer.github.com/v3/repos/branches/#get-branch
octokit.repos.getBranch({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#get-branch-protection
octokit.repos.getBranchProtection({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#update-branch-protection
octokit.repos.updateBranchProtection({
  owner,
  repo,
  branch,
  required_status_checks,
  enforce_admins,
  required_pull_request_reviews,
  restrictions,
  required_linear_history,
  allow_force_pushes,
  allow_deletions
});

// https://developer.github.com/v3/repos/branches/#remove-branch-protection
octokit.repos.removeBranchProtection({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#get-admin-enforcement-of-protected-branch
octokit.repos.getProtectedBranchAdminEnforcement({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#add-admin-enforcement-of-protected-branch
octokit.repos.addProtectedBranchAdminEnforcement({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#remove-admin-enforcement-of-protected-branch
octokit.repos.removeProtectedBranchAdminEnforcement({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#get-pull-request-review-enforcement-of-protected-branch
octokit.repos.getProtectedBranchPullRequestReviewEnforcement({
  owner,
  repo,
  branch
});

// https://developer.github.com/v3/repos/branches/#update-pull-request-review-enforcement-of-protected-branch
octokit.repos.updateProtectedBranchPullRequestReviewEnforcement({
  owner,
  repo,
  branch,
  dismissal_restrictions,
  dismiss_stale_reviews,
  require_code_owner_reviews,
  required_approving_review_count
});

// https://developer.github.com/v3/repos/branches/#remove-pull-request-review-enforcement-of-protected-branch
octokit.repos.removeProtectedBranchPullRequestReviewEnforcement({
  owner,
  repo,
  branch
});

// https://developer.github.com/v3/repos/branches/#get-required-signatures-of-protected-branch
octokit.repos.getProtectedBranchRequiredSignatures({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#add-required-signatures-of-protected-branch
octokit.repos.addProtectedBranchRequiredSignatures({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#remove-required-signatures-of-protected-branch
octokit.repos.removeProtectedBranchRequiredSignatures({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#get-required-status-checks-of-protected-branch
octokit.repos.getProtectedBranchRequiredStatusChecks({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#update-required-status-checks-of-protected-branch
octokit.repos.updateProtectedBranchRequiredStatusChecks({
  owner,
  repo,
  branch,
  strict,
  contexts
});

// https://developer.github.com/v3/repos/branches/#remove-required-status-checks-of-protected-branch
octokit.repos.removeProtectedBranchRequiredStatusChecks({
  owner,
  repo,
  branch
});

// https://developer.github.com/v3/repos/branches/#list-required-status-checks-contexts-of-protected-branch
octokit.repos.listProtectedBranchRequiredStatusChecksContexts({
  owner,
  repo,
  branch
});

// https://developer.github.com/v3/repos/branches/#replace-required-status-checks-contexts-of-protected-branch
octokit.repos.replaceProtectedBranchRequiredStatusChecksContexts({
  owner,
  repo,
  branch,
  contexts
});

// https://developer.github.com/v3/repos/branches/#add-required-status-checks-contexts-of-protected-branch
octokit.repos.addProtectedBranchRequiredStatusChecksContexts({
  owner,
  repo,
  branch,
  contexts
});

// https://developer.github.com/v3/repos/branches/#remove-required-status-checks-contexts-of-protected-branch
octokit.repos.removeProtectedBranchRequiredStatusChecksContexts({
  owner,
  repo,
  branch,
  contexts
});

// https://developer.github.com/v3/repos/branches/#get-restrictions-of-protected-branch
octokit.repos.getProtectedBranchRestrictions({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#remove-restrictions-of-protected-branch
octokit.repos.removeProtectedBranchRestrictions({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#list-apps-with-access-to-protected-branch
octokit.repos.getAppsWithAccessToProtectedBranch({ owner, repo, branch });

// DEPRECATED: octokit.repos.listAppsWithAccessToProtectedBranch() has been renamed to octokit.repos.getAppsWithAccessToProtectedBranch()
octokit.repos.listAppsWithAccessToProtectedBranch({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#replace-app-restrictions-of-protected-branch
octokit.repos.replaceProtectedBranchAppRestrictions({
  owner,
  repo,
  branch,
  apps
});

// https://developer.github.com/v3/repos/branches/#add-app-restrictions-of-protected-branch
octokit.repos.addProtectedBranchAppRestrictions({ owner, repo, branch, apps });

// https://developer.github.com/v3/repos/branches/#remove-app-restrictions-of-protected-branch
octokit.repos.removeProtectedBranchAppRestrictions({
  owner,
  repo,
  branch,
  apps
});

// https://developer.github.com/v3/repos/branches/#list-teams-with-access-to-protected-branch
octokit.repos.getTeamsWithAccessToProtectedBranch({ owner, repo, branch });

// DEPRECATED: octokit.repos.listProtectedBranchTeamRestrictions() has been renamed to octokit.repos.getTeamsWithAccessToProtectedBranch()
octokit.repos.listProtectedBranchTeamRestrictions({ owner, repo, branch });

// DEPRECATED: octokit.repos.listTeamsWithAccessToProtectedBranch() has been renamed to octokit.repos.getTeamsWithAccessToProtectedBranch()
octokit.repos.listTeamsWithAccessToProtectedBranch({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#replace-team-restrictions-of-protected-branch
octokit.repos.replaceProtectedBranchTeamRestrictions({
  owner,
  repo,
  branch,
  teams
});

// https://developer.github.com/v3/repos/branches/#add-team-restrictions-of-protected-branch
octokit.repos.addProtectedBranchTeamRestrictions({
  owner,
  repo,
  branch,
  teams
});

// https://developer.github.com/v3/repos/branches/#remove-team-restrictions-of-protected-branch
octokit.repos.removeProtectedBranchTeamRestrictions({
  owner,
  repo,
  branch,
  teams
});

// https://developer.github.com/v3/repos/branches/#list-users-with-access-to-protected-branch
octokit.repos.getUsersWithAccessToProtectedBranch({ owner, repo, branch });

// DEPRECATED: octokit.repos.listProtectedBranchUserRestrictions() has been renamed to octokit.repos.getUsersWithAccessToProtectedBranch()
octokit.repos.listProtectedBranchUserRestrictions({ owner, repo, branch });

// DEPRECATED: octokit.repos.listUsersWithAccessToProtectedBranch() has been renamed to octokit.repos.getUsersWithAccessToProtectedBranch()
octokit.repos.listUsersWithAccessToProtectedBranch({ owner, repo, branch });

// https://developer.github.com/v3/repos/branches/#replace-user-restrictions-of-protected-branch
octokit.repos.replaceProtectedBranchUserRestrictions({
  owner,
  repo,
  branch,
  users
});

// https://developer.github.com/v3/repos/branches/#add-user-restrictions-of-protected-branch
octokit.repos.addProtectedBranchUserRestrictions({
  owner,
  repo,
  branch,
  users
});

// https://developer.github.com/v3/repos/branches/#remove-user-restrictions-of-protected-branch
octokit.repos.removeProtectedBranchUserRestrictions({
  owner,
  repo,
  branch,
  users
});

// https://developer.github.com/v3/checks/runs/#create-a-check-run
octokit.checks.create({
  owner,
  repo,
  name,
  head_sha,
  details_url,
  external_id,
  status,
  started_at,
  conclusion,
  completed_at,
  output,
  actions
});

// https://developer.github.com/v3/checks/runs/#update-a-check-run
octokit.checks.update({
  owner,
  repo,
  check_run_id,
  name,
  details_url,
  external_id,
  started_at,
  status,
  conclusion,
  completed_at,
  output,
  actions
});

// https://developer.github.com/v3/checks/runs/#get-a-single-check-run
octokit.checks.get({ owner, repo, check_run_id });

// https://developer.github.com/v3/checks/runs/#list-annotations-for-a-check-run
octokit.checks.listAnnotations({ owner, repo, check_run_id });

// https://developer.github.com/v3/checks/suites/#create-a-check-suite
octokit.checks.createSuite({ owner, repo, head_sha });

// https://developer.github.com/v3/checks/suites/#set-preferences-for-check-suites-on-a-repository
octokit.checks.setSuitesPreferences({ owner, repo, auto_trigger_checks });

// https://developer.github.com/v3/checks/suites/#get-a-single-check-suite
octokit.checks.getSuite({ owner, repo, check_suite_id });

// https://developer.github.com/v3/checks/runs/#list-check-runs-in-a-check-suite
octokit.checks.listForSuite({
  owner,
  repo,
  check_suite_id,
  check_name,
  status,
  filter
});

// https://developer.github.com/v3/checks/suites/#rerequest-check-suite
octokit.checks.rerequestSuite({ owner, repo, check_suite_id });

// https://developer.github.com/v3/repos/collaborators/#list-collaborators
octokit.repos.listCollaborators({ owner, repo, affiliation });

// https://developer.github.com/v3/repos/collaborators/#check-if-a-user-is-a-collaborator
octokit.repos.checkCollaborator({ owner, repo, username });

// https://developer.github.com/v3/repos/collaborators/#add-user-as-a-collaborator
octokit.repos.addCollaborator({ owner, repo, username, permission });

// https://developer.github.com/v3/repos/collaborators/#remove-user-as-a-collaborator
octokit.repos.removeCollaborator({ owner, repo, username });

// https://developer.github.com/v3/repos/collaborators/#review-a-users-permission-level
octokit.repos.getCollaboratorPermissionLevel({ owner, repo, username });

// https://developer.github.com/v3/repos/comments/#list-commit-comments-for-a-repository
octokit.repos.listCommitComments({ owner, repo });

// https://developer.github.com/v3/repos/comments/#get-a-single-commit-comment
octokit.repos.getCommitComment({ owner, repo, comment_id });

// https://developer.github.com/v3/repos/comments/#update-a-commit-comment
octokit.repos.updateCommitComment({ owner, repo, comment_id, body });

// https://developer.github.com/v3/repos/comments/#delete-a-commit-comment
octokit.repos.deleteCommitComment({ owner, repo, comment_id });

// https://developer.github.com/v3/reactions/#list-reactions-for-a-commit-comment
octokit.reactions.listForCommitComment({ owner, repo, comment_id, content });

// https://developer.github.com/v3/reactions/#create-reaction-for-a-commit-comment
octokit.reactions.createForCommitComment({ owner, repo, comment_id, content });

// https://developer.github.com/v3/repos/commits/#list-commits-on-a-repository
octokit.repos.listCommits({ owner, repo, sha, path, author, since, until });

// https://developer.github.com/v3/repos/commits/#list-branches-for-head-commit
octokit.repos.listBranchesForHeadCommit({ owner, repo, commit_sha });

// https://developer.github.com/v3/repos/comments/#list-comments-for-a-single-commit
octokit.repos.listCommentsForCommit({ owner, repo, commit_sha });

// https://developer.github.com/v3/repos/comments/#create-a-commit-comment
octokit.repos.createCommitComment({
  owner,
  repo,
  commit_sha,
  body,
  path,
  position,
  line
});

// https://developer.github.com/v3/repos/commits/#list-pull-requests-associated-with-commit
octokit.repos.listPullRequestsAssociatedWithCommit({ owner, repo, commit_sha });

// https://developer.github.com/v3/repos/commits/#get-a-single-commit
octokit.repos.getCommit({ owner, repo, ref });

// https://developer.github.com/v3/checks/runs/#list-check-runs-for-a-specific-ref
octokit.checks.listForRef({ owner, repo, ref, check_name, status, filter });

// https://developer.github.com/v3/checks/suites/#list-check-suites-for-a-specific-ref
octokit.checks.listSuitesForRef({ owner, repo, ref, app_id, check_name });

// https://developer.github.com/v3/repos/statuses/#get-the-combined-status-for-a-specific-ref
octokit.repos.getCombinedStatusForRef({ owner, repo, ref });

// https://developer.github.com/v3/repos/statuses/#list-statuses-for-a-specific-ref
octokit.repos.listStatusesForRef({ owner, repo, ref });

// https://developer.github.com/v3/codes_of_conduct/#get-the-contents-of-a-repositorys-code-of-conduct
octokit.codesOfConduct.getForRepo({ owner, repo });

// https://developer.github.com/v3/repos/community/#retrieve-community-profile-metrics
octokit.repos.retrieveCommunityProfileMetrics({ owner, repo });

// https://developer.github.com/v3/repos/commits/#compare-two-commits
octokit.repos.compareCommits({ owner, repo, base, head });

// https://developer.github.com/v3/repos/contents/#get-contents
octokit.repos.getContents({ owner, repo, path, ref });

// https://developer.github.com/v3/repos/contents/#create-or-update-a-file
octokit.repos.createOrUpdateFile({
  owner,
  repo,
  path,
  message,
  content,
  sha,
  branch,
  committer,
  author
});

// DEPRECATED: octokit.repos.createFile() has been renamed to octokit.repos.createOrUpdateFile()
octokit.repos.createFile({
  owner,
  repo,
  path,
  message,
  content,
  sha,
  branch,
  committer,
  author
});

// DEPRECATED: octokit.repos.updateFile() has been renamed to octokit.repos.createOrUpdateFile()
octokit.repos.updateFile({
  owner,
  repo,
  path,
  message,
  content,
  sha,
  branch,
  committer,
  author
});

// https://developer.github.com/v3/repos/contents/#delete-a-file
octokit.repos.deleteFile({
  owner,
  repo,
  path,
  message,
  sha,
  branch,
  committer,
  author
});

// https://developer.github.com/v3/repos/#list-contributors
octokit.repos.listContributors({ owner, repo, anon });

// https://developer.github.com/v3/repos/deployments/#list-deployments
octokit.repos.listDeployments({ owner, repo, sha, ref, task, environment });

// https://developer.github.com/v3/repos/deployments/#create-a-deployment
octokit.repos.createDeployment({
  owner,
  repo,
  ref,
  task,
  auto_merge,
  required_contexts,
  payload,
  environment,
  description,
  transient_environment,
  production_environment
});

// https://developer.github.com/v3/repos/deployments/#get-a-single-deployment
octokit.repos.getDeployment({ owner, repo, deployment_id });

// https://developer.github.com/v3/repos/deployments/#list-deployment-statuses
octokit.repos.listDeploymentStatuses({ owner, repo, deployment_id });

// https://developer.github.com/v3/repos/deployments/#create-a-deployment-status
octokit.repos.createDeploymentStatus({
  owner,
  repo,
  deployment_id,
  state,
  target_url,
  log_url,
  description,
  environment,
  environment_url,
  auto_inactive
});

// https://developer.github.com/v3/repos/deployments/#get-a-single-deployment-status
octokit.repos.getDeploymentStatus({ owner, repo, deployment_id, status_id });

// https://developer.github.com/v3/repos/#create-a-repository-dispatch-event
octokit.repos.createDispatchEvent({ owner, repo, event_type, client_payload });

// https://developer.github.com/v3/repos/downloads/#list-downloads-for-a-repository
octokit.repos.listDownloads({ owner, repo });

// https://developer.github.com/v3/repos/downloads/#get-a-single-download
octokit.repos.getDownload({ owner, repo, download_id });

// https://developer.github.com/v3/repos/downloads/#delete-a-download
octokit.repos.deleteDownload({ owner, repo, download_id });

// https://developer.github.com/v3/activity/events/#list-repository-events
octokit.activity.listRepoEvents({ owner, repo });

// https://developer.github.com/v3/repos/forks/#list-forks
octokit.repos.listForks({ owner, repo, sort });

// https://developer.github.com/v3/repos/forks/#create-a-fork
octokit.repos.createFork({ owner, repo, organization });

// https://developer.github.com/v3/git/blobs/#create-a-blob
octokit.git.createBlob({ owner, repo, content, encoding });

// https://developer.github.com/v3/git/blobs/#get-a-blob
octokit.git.getBlob({ owner, repo, file_sha });

// https://developer.github.com/v3/git/commits/#create-a-commit
octokit.git.createCommit({
  owner,
  repo,
  message,
  tree,
  parents,
  author,
  committer,
  signature
});

// https://developer.github.com/v3/git/commits/#get-a-commit
octokit.git.getCommit({ owner, repo, commit_sha });

// https://developer.github.com/v3/git/refs/#list-matching-references
octokit.git.listMatchingRefs({ owner, repo, ref });

// https://developer.github.com/v3/git/refs/#get-a-single-reference
octokit.git.getRef({ owner, repo, ref });

// https://developer.github.com/v3/git/refs/#create-a-reference
octokit.git.createRef({ owner, repo, ref, sha });

// https://developer.github.com/v3/git/refs/#update-a-reference
octokit.git.updateRef({ owner, repo, ref, sha, force });

// https://developer.github.com/v3/git/refs/#delete-a-reference
octokit.git.deleteRef({ owner, repo, ref });

// https://developer.github.com/v3/git/tags/#create-a-tag-object
octokit.git.createTag({ owner, repo, tag, message, object, type, tagger });

// https://developer.github.com/v3/git/tags/#get-a-tag
octokit.git.getTag({ owner, repo, tag_sha });

// https://developer.github.com/v3/git/trees/#create-a-tree
octokit.git.createTree({ owner, repo, tree, base_tree });

// https://developer.github.com/v3/git/trees/#get-a-tree
octokit.git.getTree({ owner, repo, tree_sha, recursive });

// https://developer.github.com/v3/repos/hooks/#list-hooks
octokit.repos.listHooks({ owner, repo });

// https://developer.github.com/v3/repos/hooks/#create-a-hook
octokit.repos.createHook({ owner, repo, name, config, events, active });

// https://developer.github.com/v3/repos/hooks/#get-single-hook
octokit.repos.getHook({ owner, repo, hook_id });

// https://developer.github.com/v3/repos/hooks/#edit-a-hook
octokit.repos.updateHook({
  owner,
  repo,
  hook_id,
  config,
  events,
  add_events,
  remove_events,
  active
});

// https://developer.github.com/v3/repos/hooks/#delete-a-hook
octokit.repos.deleteHook({ owner, repo, hook_id });

// https://developer.github.com/v3/repos/hooks/#ping-a-hook
octokit.repos.pingHook({ owner, repo, hook_id });

// https://developer.github.com/v3/repos/hooks/#test-a-push-hook
octokit.repos.testPushHook({ owner, repo, hook_id });

// https://developer.github.com/v3/migrations/source_imports/#start-an-import
octokit.migrations.startImport({
  owner,
  repo,
  vcs_url,
  vcs,
  vcs_username,
  vcs_password,
  tfvc_project
});

// https://developer.github.com/v3/migrations/source_imports/#get-import-progress
octokit.migrations.getImportProgress({ owner, repo });

// https://developer.github.com/v3/migrations/source_imports/#update-existing-import
octokit.migrations.updateImport({ owner, repo, vcs_username, vcs_password });

// https://developer.github.com/v3/migrations/source_imports/#cancel-an-import
octokit.migrations.cancelImport({ owner, repo });

// https://developer.github.com/v3/migrations/source_imports/#get-commit-authors
octokit.migrations.getCommitAuthors({ owner, repo, since });

// https://developer.github.com/v3/migrations/source_imports/#map-a-commit-author
octokit.migrations.mapCommitAuthor({ owner, repo, author_id, email, name });

// https://developer.github.com/v3/migrations/source_imports/#get-large-files
octokit.migrations.getLargeFiles({ owner, repo });

// https://developer.github.com/v3/migrations/source_imports/#set-git-lfs-preference
octokit.migrations.setLfsPreference({ owner, repo, use_lfs });

// https://developer.github.com/v3/apps/#get-a-repository-installation
octokit.apps.getRepoInstallation({ owner, repo });

// DEPRECATED: octokit.apps.findRepoInstallation() has been renamed to octokit.apps.getRepoInstallation()
octokit.apps.findRepoInstallation({ owner, repo });

// https://developer.github.com/v3/interactions/repos/#get-interaction-restrictions-for-a-repository
octokit.interactions.getRestrictionsForRepo({ owner, repo });

// https://developer.github.com/v3/interactions/repos/#add-or-update-interaction-restrictions-for-a-repository
octokit.interactions.addOrUpdateRestrictionsForRepo({ owner, repo, limit });

// https://developer.github.com/v3/interactions/repos/#remove-interaction-restrictions-for-a-repository
octokit.interactions.removeRestrictionsForRepo({ owner, repo });

// https://developer.github.com/v3/repos/invitations/#list-invitations-for-a-repository
octokit.repos.listInvitations({ owner, repo });

// https://developer.github.com/v3/repos/invitations/#delete-a-repository-invitation
octokit.repos.deleteInvitation({ owner, repo, invitation_id });

// https://developer.github.com/v3/repos/invitations/#update-a-repository-invitation
octokit.repos.updateInvitation({ owner, repo, invitation_id, permissions });

// https://developer.github.com/v3/issues/#list-issues-for-a-repository
octokit.issues.listForRepo({
  owner,
  repo,
  milestone,
  state,
  assignee,
  creator,
  mentioned,
  labels,
  sort,
  direction,
  since
});

// https://developer.github.com/v3/issues/#create-an-issue
octokit.issues.create({
  owner,
  repo,
  title,
  body,
  assignee,
  milestone,
  labels,
  assignees
});

// https://developer.github.com/v3/issues/comments/#list-comments-in-a-repository
octokit.issues.listCommentsForRepo({ owner, repo, sort, direction, since });

// https://developer.github.com/v3/issues/comments/#get-a-single-comment
octokit.issues.getComment({ owner, repo, comment_id });

// https://developer.github.com/v3/issues/comments/#edit-a-comment
octokit.issues.updateComment({ owner, repo, comment_id, body });

// https://developer.github.com/v3/issues/comments/#delete-a-comment
octokit.issues.deleteComment({ owner, repo, comment_id });

// https://developer.github.com/v3/reactions/#list-reactions-for-an-issue-comment
octokit.reactions.listForIssueComment({ owner, repo, comment_id, content });

// https://developer.github.com/v3/reactions/#create-reaction-for-an-issue-comment
octokit.reactions.createForIssueComment({ owner, repo, comment_id, content });

// https://developer.github.com/v3/issues/events/#list-events-for-a-repository
octokit.issues.listEventsForRepo({ owner, repo });

// https://developer.github.com/v3/issues/events/#get-a-single-event
octokit.issues.getEvent({ owner, repo, event_id });

// https://developer.github.com/v3/issues/#get-a-single-issue
octokit.issues.get({ owner, repo, issue_number });

// https://developer.github.com/v3/issues/#edit-an-issue
octokit.issues.update({
  owner,
  repo,
  issue_number,
  title,
  body,
  assignee,
  state,
  milestone,
  labels,
  assignees
});

// https://developer.github.com/v3/issues/assignees/#add-assignees-to-an-issue
octokit.issues.addAssignees({ owner, repo, issue_number, assignees });

// https://developer.github.com/v3/issues/assignees/#remove-assignees-from-an-issue
octokit.issues.removeAssignees({ owner, repo, issue_number, assignees });

// https://developer.github.com/v3/issues/comments/#list-comments-on-an-issue
octokit.issues.listComments({ owner, repo, issue_number, since });

// https://developer.github.com/v3/issues/comments/#create-a-comment
octokit.issues.createComment({ owner, repo, issue_number, body });

// https://developer.github.com/v3/issues/events/#list-events-for-an-issue
octokit.issues.listEvents({ owner, repo, issue_number });

// https://developer.github.com/v3/issues/labels/#list-labels-on-an-issue
octokit.issues.listLabelsOnIssue({ owner, repo, issue_number });

// https://developer.github.com/v3/issues/labels/#add-labels-to-an-issue
octokit.issues.addLabels({ owner, repo, issue_number, labels });

// https://developer.github.com/v3/issues/labels/#replace-all-labels-for-an-issue
octokit.issues.replaceLabels({ owner, repo, issue_number, labels });

// https://developer.github.com/v3/issues/labels/#remove-all-labels-from-an-issue
octokit.issues.removeLabels({ owner, repo, issue_number });

// https://developer.github.com/v3/issues/labels/#remove-a-label-from-an-issue
octokit.issues.removeLabel({ owner, repo, issue_number, name });

// https://developer.github.com/v3/issues/#lock-an-issue
octokit.issues.lock({ owner, repo, issue_number, lock_reason });

// https://developer.github.com/v3/issues/#unlock-an-issue
octokit.issues.unlock({ owner, repo, issue_number });

// https://developer.github.com/v3/reactions/#list-reactions-for-an-issue
octokit.reactions.listForIssue({ owner, repo, issue_number, content });

// https://developer.github.com/v3/reactions/#create-reaction-for-an-issue
octokit.reactions.createForIssue({ owner, repo, issue_number, content });

// https://developer.github.com/v3/issues/timeline/#list-events-for-an-issue
octokit.issues.listEventsForTimeline({ owner, repo, issue_number });

// https://developer.github.com/v3/repos/keys/#list-deploy-keys
octokit.repos.listDeployKeys({ owner, repo });

// https://developer.github.com/v3/repos/keys/#add-a-new-deploy-key
octokit.repos.addDeployKey({ owner, repo, title, key, read_only });

// https://developer.github.com/v3/repos/keys/#get-a-deploy-key
octokit.repos.getDeployKey({ owner, repo, key_id });

// https://developer.github.com/v3/repos/keys/#remove-a-deploy-key
octokit.repos.removeDeployKey({ owner, repo, key_id });

// https://developer.github.com/v3/issues/labels/#list-all-labels-for-this-repository
octokit.issues.listLabelsForRepo({ owner, repo });

// https://developer.github.com/v3/issues/labels/#create-a-label
octokit.issues.createLabel({ owner, repo, name, color, description });

// https://developer.github.com/v3/issues/labels/#get-a-single-label
octokit.issues.getLabel({ owner, repo, name });

// https://developer.github.com/v3/issues/labels/#update-a-label
octokit.issues.updateLabel({ owner, repo, name, new_name, color, description });

// https://developer.github.com/v3/issues/labels/#delete-a-label
octokit.issues.deleteLabel({ owner, repo, name });

// https://developer.github.com/v3/repos/#list-languages
octokit.repos.listLanguages({ owner, repo });

// https://developer.github.com/v3/licenses/#get-the-contents-of-a-repositorys-license
octokit.licenses.getForRepo({ owner, repo });

// https://developer.github.com/v3/repos/merging/#perform-a-merge
octokit.repos.merge({ owner, repo, base, head, commit_message });

// https://developer.github.com/v3/issues/milestones/#list-milestones-for-a-repository
octokit.issues.listMilestonesForRepo({ owner, repo, state, sort, direction });

// https://developer.github.com/v3/issues/milestones/#create-a-milestone
octokit.issues.createMilestone({
  owner,
  repo,
  title,
  state,
  description,
  due_on
});

// https://developer.github.com/v3/issues/milestones/#get-a-single-milestone
octokit.issues.getMilestone({ owner, repo, milestone_number });

// https://developer.github.com/v3/issues/milestones/#update-a-milestone
octokit.issues.updateMilestone({
  owner,
  repo,
  milestone_number,
  title,
  state,
  description,
  due_on
});

// https://developer.github.com/v3/issues/milestones/#delete-a-milestone
octokit.issues.deleteMilestone({ owner, repo, milestone_number });

// https://developer.github.com/v3/issues/labels/#get-labels-for-every-issue-in-a-milestone
octokit.issues.listLabelsForMilestone({ owner, repo, milestone_number });

// https://developer.github.com/v3/activity/notifications/#list-your-notifications-in-a-repository
octokit.activity.listNotificationsForRepo({
  owner,
  repo,
  all,
  participating,
  since,
  before
});

// https://developer.github.com/v3/activity/notifications/#mark-notifications-as-read-in-a-repository
octokit.activity.markNotificationsAsReadForRepo({ owner, repo, last_read_at });

// https://developer.github.com/v3/repos/pages/#get-information-about-a-pages-site
octokit.repos.getPages({ owner, repo });

// https://developer.github.com/v3/repos/pages/#enable-a-pages-site
octokit.repos.enablePagesSite({ owner, repo, source });

// https://developer.github.com/v3/repos/pages/#disable-a-pages-site
octokit.repos.disablePagesSite({ owner, repo });

// https://developer.github.com/v3/repos/pages/#update-information-about-a-pages-site
octokit.repos.updateInformationAboutPagesSite({ owner, repo, cname, source });

// https://developer.github.com/v3/repos/pages/#request-a-page-build
octokit.repos.requestPageBuild({ owner, repo });

// https://developer.github.com/v3/repos/pages/#list-pages-builds
octokit.repos.listPagesBuilds({ owner, repo });

// https://developer.github.com/v3/repos/pages/#get-latest-pages-build
octokit.repos.getLatestPagesBuild({ owner, repo });

// https://developer.github.com/v3/repos/pages/#get-a-specific-pages-build
octokit.repos.getPagesBuild({ owner, repo, build_id });

// https://developer.github.com/v3/projects/#list-repository-projects
octokit.projects.listForRepo({ owner, repo, state });

// https://developer.github.com/v3/projects/#create-a-repository-project
octokit.projects.createForRepo({ owner, repo, name, body });

// https://developer.github.com/v3/pulls/#list-pull-requests
octokit.pulls.list({ owner, repo, state, head, base, sort, direction });

// https://developer.github.com/v3/pulls/#create-a-pull-request
octokit.pulls.create({
  owner,
  repo,
  title,
  head,
  base,
  body,
  maintainer_can_modify,
  draft
});

// https://developer.github.com/v3/pulls/comments/#list-comments-in-a-repository
octokit.pulls.listCommentsForRepo({ owner, repo, sort, direction, since });

// https://developer.github.com/v3/pulls/comments/#get-a-single-comment
octokit.pulls.getComment({ owner, repo, comment_id });

// https://developer.github.com/v3/pulls/comments/#edit-a-comment
octokit.pulls.updateComment({ owner, repo, comment_id, body });

// https://developer.github.com/v3/pulls/comments/#delete-a-comment
octokit.pulls.deleteComment({ owner, repo, comment_id });

// https://developer.github.com/v3/reactions/#list-reactions-for-a-pull-request-review-comment
octokit.reactions.listForPullRequestReviewComment({
  owner,
  repo,
  comment_id,
  content
});

// https://developer.github.com/v3/reactions/#create-reaction-for-a-pull-request-review-comment
octokit.reactions.createForPullRequestReviewComment({
  owner,
  repo,
  comment_id,
  content
});

// https://developer.github.com/v3/pulls/#get-a-single-pull-request
octokit.pulls.get({ owner, repo, pull_number });

// https://developer.github.com/v3/pulls/#update-a-pull-request
octokit.pulls.update({
  owner,
  repo,
  pull_number,
  title,
  body,
  state,
  base,
  maintainer_can_modify
});

// https://developer.github.com/v3/pulls/comments/#list-comments-on-a-pull-request
octokit.pulls.listComments({
  owner,
  repo,
  pull_number,
  sort,
  direction,
  since
});

// https://developer.github.com/v3/pulls/comments/#create-a-comment
octokit.pulls.createComment({
  owner,
  repo,
  pull_number,
  body,
  commit_id,
  path,
  position,
  side,
  line,
  start_line,
  start_side,
  in_reply_to
});

// DEPRECATED: octokit.pulls.createCommentReply() has been renamed to octokit.pulls.createComment()
octokit.pulls.createCommentReply({
  owner,
  repo,
  pull_number,
  body,
  commit_id,
  path,
  position,
  side,
  line,
  start_line,
  start_side,
  in_reply_to
});

// https://developer.github.com/v3/pulls/comments/#create-a-review-comment-reply
octokit.pulls.createReviewCommentReply({
  owner,
  repo,
  pull_number,
  comment_id,
  body
});

// https://developer.github.com/v3/pulls/#list-commits-on-a-pull-request
octokit.pulls.listCommits({ owner, repo, pull_number });

// https://developer.github.com/v3/pulls/#list-pull-requests-files
octokit.pulls.listFiles({ owner, repo, pull_number });

// https://developer.github.com/v3/pulls/#get-if-a-pull-request-has-been-merged
octokit.pulls.checkIfMerged({ owner, repo, pull_number });

// https://developer.github.com/v3/pulls/#merge-a-pull-request-merge-button
octokit.pulls.merge({
  owner,
  repo,
  pull_number,
  commit_title,
  commit_message,
  sha,
  merge_method
});

// https://developer.github.com/v3/pulls/review_requests/#list-review-requests
octokit.pulls.listReviewRequests({ owner, repo, pull_number });

// https://developer.github.com/v3/pulls/review_requests/#create-a-review-request
octokit.pulls.createReviewRequest({
  owner,
  repo,
  pull_number,
  reviewers,
  team_reviewers
});

// https://developer.github.com/v3/pulls/review_requests/#delete-a-review-request
octokit.pulls.deleteReviewRequest({
  owner,
  repo,
  pull_number,
  reviewers,
  team_reviewers
});

// https://developer.github.com/v3/pulls/reviews/#list-reviews-on-a-pull-request
octokit.pulls.listReviews({ owner, repo, pull_number });

// https://developer.github.com/v3/pulls/reviews/#create-a-pull-request-review
octokit.pulls.createReview({
  owner,
  repo,
  pull_number,
  commit_id,
  body,
  event,
  comments
});

// https://developer.github.com/v3/pulls/reviews/#get-a-single-review
octokit.pulls.getReview({ owner, repo, pull_number, review_id });

// https://developer.github.com/v3/pulls/reviews/#delete-a-pending-review
octokit.pulls.deletePendingReview({ owner, repo, pull_number, review_id });

// https://developer.github.com/v3/pulls/reviews/#update-a-pull-request-review
octokit.pulls.updateReview({ owner, repo, pull_number, review_id, body });

// https://developer.github.com/v3/pulls/reviews/#get-comments-for-a-single-review
octokit.pulls.getCommentsForReview({ owner, repo, pull_number, review_id });

// https://developer.github.com/v3/pulls/reviews/#dismiss-a-pull-request-review
octokit.pulls.dismissReview({ owner, repo, pull_number, review_id, message });

// https://developer.github.com/v3/pulls/reviews/#submit-a-pull-request-review
octokit.pulls.submitReview({
  owner,
  repo,
  pull_number,
  review_id,
  body,
  event
});

// https://developer.github.com/v3/pulls/#update-a-pull-request-branch
octokit.pulls.updateBranch({ owner, repo, pull_number, expected_head_sha });

// https://developer.github.com/v3/repos/contents/#get-the-readme
octokit.repos.getReadme({ owner, repo, ref });

// https://developer.github.com/v3/repos/releases/#list-releases-for-a-repository
octokit.repos.listReleases({ owner, repo });

// https://developer.github.com/v3/repos/releases/#create-a-release
octokit.repos.createRelease({
  owner,
  repo,
  tag_name,
  target_commitish,
  name,
  body,
  draft,
  prerelease
});

// https://developer.github.com/v3/repos/releases/#get-a-single-release-asset
octokit.repos.getReleaseAsset({ owner, repo, asset_id });

// https://developer.github.com/v3/repos/releases/#edit-a-release-asset
octokit.repos.updateReleaseAsset({ owner, repo, asset_id, name, label });

// https://developer.github.com/v3/repos/releases/#delete-a-release-asset
octokit.repos.deleteReleaseAsset({ owner, repo, asset_id });

// https://developer.github.com/v3/repos/releases/#get-the-latest-release
octokit.repos.getLatestRelease({ owner, repo });

// https://developer.github.com/v3/repos/releases/#get-a-release-by-tag-name
octokit.repos.getReleaseByTag({ owner, repo, tag });

// https://developer.github.com/v3/repos/releases/#get-a-single-release
octokit.repos.getRelease({ owner, repo, release_id });

// https://developer.github.com/v3/repos/releases/#edit-a-release
octokit.repos.updateRelease({
  owner,
  repo,
  release_id,
  tag_name,
  target_commitish,
  name,
  body,
  draft,
  prerelease
});

// https://developer.github.com/v3/repos/releases/#delete-a-release
octokit.repos.deleteRelease({ owner, repo, release_id });

// https://developer.github.com/v3/repos/releases/#list-assets-for-a-release
octokit.repos.listAssetsForRelease({ owner, repo, release_id });

// https://developer.github.com/v3/repos/releases/#upload-a-release-asset
octokit.repos.uploadReleaseAsset({
  owner,
  repo,
  release_id,
  name,
  label,
  data,
  origin
});

// https://developer.github.com/v3/activity/starring/#list-stargazers
octokit.activity.listStargazersForRepo({ owner, repo });

// https://developer.github.com/v3/repos/statistics/#get-the-number-of-additions-and-deletions-per-week
octokit.repos.getCodeFrequencyStats({ owner, repo });

// https://developer.github.com/v3/repos/statistics/#get-the-last-year-of-commit-activity-data
octokit.repos.getCommitActivityStats({ owner, repo });

// https://developer.github.com/v3/repos/statistics/#get-contributors-list-with-additions-deletions-and-commit-counts
octokit.repos.getContributorsStats({ owner, repo });

// https://developer.github.com/v3/repos/statistics/#get-the-weekly-commit-count-for-the-repository-owner-and-everyone-else
octokit.repos.getParticipationStats({ owner, repo });

// https://developer.github.com/v3/repos/statistics/#get-the-number-of-commits-per-hour-in-each-day
octokit.repos.getPunchCardStats({ owner, repo });

// https://developer.github.com/v3/repos/statuses/#create-a-status
octokit.repos.createStatus({
  owner,
  repo,
  sha,
  state,
  target_url,
  description,
  context
});

// https://developer.github.com/v3/activity/watching/#list-watchers
octokit.activity.listWatchersForRepo({ owner, repo });

// https://developer.github.com/v3/activity/watching/#get-a-repository-subscription
octokit.activity.getRepoSubscription({ owner, repo });

// https://developer.github.com/v3/activity/watching/#set-a-repository-subscription
octokit.activity.setRepoSubscription({ owner, repo, subscribed, ignored });

// https://developer.github.com/v3/activity/watching/#delete-a-repository-subscription
octokit.activity.deleteRepoSubscription({ owner, repo });

// https://developer.github.com/v3/repos/#list-tags
octokit.repos.listTags({ owner, repo });

// https://developer.github.com/v3/repos/#list-teams
octokit.repos.listTeams({ owner, repo });

// https://developer.github.com/v3/repos/#list-all-topics-for-a-repository
octokit.repos.listTopics({ owner, repo });

// https://developer.github.com/v3/repos/#replace-all-topics-for-a-repository
octokit.repos.replaceTopics({ owner, repo, names });

// https://developer.github.com/v3/repos/traffic/#clones
octokit.repos.getClones({ owner, repo, per });

// https://developer.github.com/v3/repos/traffic/#list-paths
octokit.repos.getTopPaths({ owner, repo });

// https://developer.github.com/v3/repos/traffic/#list-referrers
octokit.repos.getTopReferrers({ owner, repo });

// https://developer.github.com/v3/repos/traffic/#views
octokit.repos.getViews({ owner, repo, per });

// https://developer.github.com/v3/repos/#transfer-a-repository
octokit.repos.transfer({ owner, repo, new_owner, team_ids });

// https://developer.github.com/v3/repos/#check-if-vulnerability-alerts-are-enabled-for-a-repository
octokit.repos.checkVulnerabilityAlerts({ owner, repo });

// https://developer.github.com/v3/repos/#enable-vulnerability-alerts
octokit.repos.enableVulnerabilityAlerts({ owner, repo });

// https://developer.github.com/v3/repos/#disable-vulnerability-alerts
octokit.repos.disableVulnerabilityAlerts({ owner, repo });

// https://developer.github.com/v3/repos/contents/#get-archive-link
octokit.repos.getArchiveLink({ owner, repo, archive_format, ref });

// https://developer.github.com/v3/repos/#create-repository-using-a-repository-template
octokit.repos.createUsingTemplate({
  template_owner,
  template_repo,
  owner,
  name,
  description,
  private
});

// https://developer.github.com/v3/repos/#list-all-public-repositories
octokit.repos.listPublic({ since });

// https://developer.github.com/v3/search/#search-code
octokit.search.code({ q, sort, order });

// https://developer.github.com/v3/search/#search-commits
octokit.search.commits({ q, sort, order });

// https://developer.github.com/v3/search/#search-issues-and-pull-requests
octokit.search.issuesAndPullRequests({ q, sort, order });

// DEPRECATED: octokit.search.issues() has been renamed to octokit.search.issuesAndPullRequests()
octokit.search.issues({ q, sort, order });

// https://developer.github.com/v3/search/#search-labels
octokit.search.labels({ repository_id, q, sort, order });

// https://developer.github.com/v3/search/#search-repositories
octokit.search.repos({ q, sort, order });

// https://developer.github.com/v3/search/#search-topics
octokit.search.topics({ q });

// https://developer.github.com/v3/search/#search-users
octokit.search.users({ q, sort, order });

// https://developer.github.com/v3/teams/#get-team-legacy
octokit.teams.getLegacy({ team_id });

// DEPRECATED: octokit.teams.get() has been renamed to octokit.teams.getLegacy()
octokit.teams.get({ team_id });

// https://developer.github.com/v3/teams/#edit-team-legacy
octokit.teams.updateLegacy({
  team_id,
  name,
  description,
  privacy,
  permission,
  parent_team_id
});

// DEPRECATED: octokit.teams.update() has been renamed to octokit.teams.updateLegacy()
octokit.teams.update({
  team_id,
  name,
  description,
  privacy,
  permission,
  parent_team_id
});

// https://developer.github.com/v3/teams/#delete-team-legacy
octokit.teams.deleteLegacy({ team_id });

// DEPRECATED: octokit.teams.delete() has been renamed to octokit.teams.deleteLegacy()
octokit.teams.delete({ team_id });

// https://developer.github.com/v3/teams/discussions/#list-discussions-legacy
octokit.teams.listDiscussionsLegacy({ team_id, direction });

// DEPRECATED: octokit.teams.listDiscussions() has been renamed to octokit.teams.listDiscussionsLegacy()
octokit.teams.listDiscussions({ team_id, direction });

// https://developer.github.com/v3/teams/discussions/#create-a-discussion-legacy
octokit.teams.createDiscussionLegacy({ team_id, title, body, private });

// DEPRECATED: octokit.teams.createDiscussion() has been renamed to octokit.teams.createDiscussionLegacy()
octokit.teams.createDiscussion({ team_id, title, body, private });

// https://developer.github.com/v3/teams/discussions/#get-a-single-discussion-legacy
octokit.teams.getDiscussionLegacy({ team_id, discussion_number });

// DEPRECATED: octokit.teams.getDiscussion() has been renamed to octokit.teams.getDiscussionLegacy()
octokit.teams.getDiscussion({ team_id, discussion_number });

// https://developer.github.com/v3/teams/discussions/#edit-a-discussion-legacy
octokit.teams.updateDiscussionLegacy({
  team_id,
  discussion_number,
  title,
  body
});

// DEPRECATED: octokit.teams.updateDiscussion() has been renamed to octokit.teams.updateDiscussionLegacy()
octokit.teams.updateDiscussion({ team_id, discussion_number, title, body });

// https://developer.github.com/v3/teams/discussions/#delete-a-discussion-legacy
octokit.teams.deleteDiscussionLegacy({ team_id, discussion_number });

// DEPRECATED: octokit.teams.deleteDiscussion() has been renamed to octokit.teams.deleteDiscussionLegacy()
octokit.teams.deleteDiscussion({ team_id, discussion_number });

// https://developer.github.com/v3/teams/discussion_comments/#list-comments-legacy
octokit.teams.listDiscussionCommentsLegacy({
  team_id,
  discussion_number,
  direction
});

// DEPRECATED: octokit.teams.listDiscussionComments() has been renamed to octokit.teams.listDiscussionCommentsLegacy()
octokit.teams.listDiscussionComments({ team_id, discussion_number, direction });

// https://developer.github.com/v3/teams/discussion_comments/#create-a-comment-legacy
octokit.teams.createDiscussionCommentLegacy({
  team_id,
  discussion_number,
  body
});

// DEPRECATED: octokit.teams.createDiscussionComment() has been renamed to octokit.teams.createDiscussionCommentLegacy()
octokit.teams.createDiscussionComment({ team_id, discussion_number, body });

// https://developer.github.com/v3/teams/discussion_comments/#get-a-single-comment-legacy
octokit.teams.getDiscussionCommentLegacy({
  team_id,
  discussion_number,
  comment_number
});

// DEPRECATED: octokit.teams.getDiscussionComment() has been renamed to octokit.teams.getDiscussionCommentLegacy()
octokit.teams.getDiscussionComment({
  team_id,
  discussion_number,
  comment_number
});

// https://developer.github.com/v3/teams/discussion_comments/#edit-a-comment-legacy
octokit.teams.updateDiscussionCommentLegacy({
  team_id,
  discussion_number,
  comment_number,
  body
});

// DEPRECATED: octokit.teams.updateDiscussionComment() has been renamed to octokit.teams.updateDiscussionCommentLegacy()
octokit.teams.updateDiscussionComment({
  team_id,
  discussion_number,
  comment_number,
  body
});

// https://developer.github.com/v3/teams/discussion_comments/#delete-a-comment-legacy
octokit.teams.deleteDiscussionCommentLegacy({
  team_id,
  discussion_number,
  comment_number
});

// DEPRECATED: octokit.teams.deleteDiscussionComment() has been renamed to octokit.teams.deleteDiscussionCommentLegacy()
octokit.teams.deleteDiscussionComment({
  team_id,
  discussion_number,
  comment_number
});

// https://developer.github.com/v3/reactions/#list-reactions-for-a-team-discussion-comment-legacy
octokit.reactions.listForTeamDiscussionCommentLegacy({
  team_id,
  discussion_number,
  comment_number,
  content
});

// DEPRECATED: octokit.reactions.listForTeamDiscussionComment() has been renamed to octokit.reactions.listForTeamDiscussionCommentLegacy()
octokit.reactions.listForTeamDiscussionComment({
  team_id,
  discussion_number,
  comment_number,
  content
});

// https://developer.github.com/v3/reactions/#create-reaction-for-a-team-discussion-comment-legacy
octokit.reactions.createForTeamDiscussionCommentLegacy({
  team_id,
  discussion_number,
  comment_number,
  content
});

// DEPRECATED: octokit.reactions.createForTeamDiscussionComment() has been renamed to octokit.reactions.createForTeamDiscussionCommentLegacy()
octokit.reactions.createForTeamDiscussionComment({
  team_id,
  discussion_number,
  comment_number,
  content
});

// https://developer.github.com/v3/reactions/#list-reactions-for-a-team-discussion-legacy
octokit.reactions.listForTeamDiscussionLegacy({
  team_id,
  discussion_number,
  content
});

// DEPRECATED: octokit.reactions.listForTeamDiscussion() has been renamed to octokit.reactions.listForTeamDiscussionLegacy()
octokit.reactions.listForTeamDiscussion({
  team_id,
  discussion_number,
  content
});

// https://developer.github.com/v3/reactions/#create-reaction-for-a-team-discussion-legacy
octokit.reactions.createForTeamDiscussionLegacy({
  team_id,
  discussion_number,
  content
});

// DEPRECATED: octokit.reactions.createForTeamDiscussion() has been renamed to octokit.reactions.createForTeamDiscussionLegacy()
octokit.reactions.createForTeamDiscussion({
  team_id,
  discussion_number,
  content
});

// https://developer.github.com/v3/teams/members/#list-pending-team-invitations-legacy
octokit.teams.listPendingInvitationsLegacy({ team_id });

// DEPRECATED: octokit.teams.listPendingInvitations() has been renamed to octokit.teams.listPendingInvitationsLegacy()
octokit.teams.listPendingInvitations({ team_id });

// https://developer.github.com/v3/teams/members/#list-team-members-legacy
octokit.teams.listMembersLegacy({ team_id, role });

// DEPRECATED: octokit.teams.listMembers() has been renamed to octokit.teams.listMembersLegacy()
octokit.teams.listMembers({ team_id, role });

// https://developer.github.com/v3/teams/members/#get-team-member-legacy
octokit.teams.getMemberLegacy({ team_id, username });

// DEPRECATED: octokit.teams.getMember() has been renamed to octokit.teams.getMemberLegacy()
octokit.teams.getMember({ team_id, username });

// https://developer.github.com/v3/teams/members/#add-team-member-legacy
octokit.teams.addMemberLegacy({ team_id, username });

// DEPRECATED: octokit.teams.addMember() has been renamed to octokit.teams.addMemberLegacy()
octokit.teams.addMember({ team_id, username });

// https://developer.github.com/v3/teams/members/#remove-team-member-legacy
octokit.teams.removeMemberLegacy({ team_id, username });

// DEPRECATED: octokit.teams.removeMember() has been renamed to octokit.teams.removeMemberLegacy()
octokit.teams.removeMember({ team_id, username });

// https://developer.github.com/v3/teams/members/#get-team-membership-legacy
octokit.teams.getMembershipLegacy({ team_id, username });

// DEPRECATED: octokit.teams.getMembership() has been renamed to octokit.teams.getMembershipLegacy()
octokit.teams.getMembership({ team_id, username });

// https://developer.github.com/v3/teams/members/#add-or-update-team-membership-legacy
octokit.teams.addOrUpdateMembershipLegacy({ team_id, username, role });

// DEPRECATED: octokit.teams.addOrUpdateMembership() has been renamed to octokit.teams.addOrUpdateMembershipLegacy()
octokit.teams.addOrUpdateMembership({ team_id, username, role });

// https://developer.github.com/v3/teams/members/#remove-team-membership-legacy
octokit.teams.removeMembershipLegacy({ team_id, username });

// DEPRECATED: octokit.teams.removeMembership() has been renamed to octokit.teams.removeMembershipLegacy()
octokit.teams.removeMembership({ team_id, username });

// https://developer.github.com/v3/teams/#list-team-projects-legacy
octokit.teams.listProjectsLegacy({ team_id });

// DEPRECATED: octokit.teams.listProjects() has been renamed to octokit.teams.listProjectsLegacy()
octokit.teams.listProjects({ team_id });

// https://developer.github.com/v3/teams/#review-a-team-project-legacy
octokit.teams.reviewProjectLegacy({ team_id, project_id });

// DEPRECATED: octokit.teams.reviewProject() has been renamed to octokit.teams.reviewProjectLegacy()
octokit.teams.reviewProject({ team_id, project_id });

// https://developer.github.com/v3/teams/#add-or-update-team-project-legacy
octokit.teams.addOrUpdateProjectLegacy({ team_id, project_id, permission });

// DEPRECATED: octokit.teams.addOrUpdateProject() has been renamed to octokit.teams.addOrUpdateProjectLegacy()
octokit.teams.addOrUpdateProject({ team_id, project_id, permission });

// https://developer.github.com/v3/teams/#remove-team-project-legacy
octokit.teams.removeProjectLegacy({ team_id, project_id });

// DEPRECATED: octokit.teams.removeProject() has been renamed to octokit.teams.removeProjectLegacy()
octokit.teams.removeProject({ team_id, project_id });

// https://developer.github.com/v3/teams/#list-team-repos-legacy
octokit.teams.listReposLegacy({ team_id });

// DEPRECATED: octokit.teams.listRepos() has been renamed to octokit.teams.listReposLegacy()
octokit.teams.listRepos({ team_id });

// https://developer.github.com/v3/teams/#check-if-a-team-manages-a-repository-legacy
octokit.teams.checkManagesRepoLegacy({ team_id, owner, repo });

// DEPRECATED: octokit.teams.checkManagesRepo() has been renamed to octokit.teams.checkManagesRepoLegacy()
octokit.teams.checkManagesRepo({ team_id, owner, repo });

// https://developer.github.com/v3/teams/#add-or-update-team-repository-legacy
octokit.teams.addOrUpdateRepoLegacy({ team_id, owner, repo, permission });

// DEPRECATED: octokit.teams.addOrUpdateRepo() has been renamed to octokit.teams.addOrUpdateRepoLegacy()
octokit.teams.addOrUpdateRepo({ team_id, owner, repo, permission });

// https://developer.github.com/v3/teams/#remove-team-repository-legacy
octokit.teams.removeRepoLegacy({ team_id, owner, repo });

// DEPRECATED: octokit.teams.removeRepo() has been renamed to octokit.teams.removeRepoLegacy()
octokit.teams.removeRepo({ team_id, owner, repo });

// https://developer.github.com/v3/teams/#list-child-teams-legacy
octokit.teams.listChildLegacy({ team_id });

// DEPRECATED: octokit.teams.listChild() has been renamed to octokit.teams.listChildLegacy()
octokit.teams.listChild({ team_id });

// https://developer.github.com/v3/users/#get-the-authenticated-user
octokit.users.getAuthenticated();

// https://developer.github.com/v3/users/#update-the-authenticated-user
octokit.users.updateAuthenticated({
  name,
  email,
  blog,
  company,
  location,
  hireable,
  bio
});

// https://developer.github.com/v3/users/blocking/#list-blocked-users
octokit.users.listBlocked();

// https://developer.github.com/v3/users/blocking/#check-whether-youve-blocked-a-user
octokit.users.checkBlocked({ username });

// https://developer.github.com/v3/users/blocking/#block-a-user
octokit.users.block({ username });

// https://developer.github.com/v3/users/blocking/#unblock-a-user
octokit.users.unblock({ username });

// https://developer.github.com/v3/users/emails/#toggle-primary-email-visibility
octokit.users.togglePrimaryEmailVisibility({ email, visibility });

// https://developer.github.com/v3/users/emails/#list-email-addresses-for-a-user
octokit.users.listEmails();

// https://developer.github.com/v3/users/emails/#add-email-addresses
octokit.users.addEmails({ emails });

// https://developer.github.com/v3/users/emails/#delete-email-addresses
octokit.users.deleteEmails({ emails });

// https://developer.github.com/v3/users/followers/#list-followers-of-a-user
octokit.users.listFollowersForAuthenticatedUser();

// https://developer.github.com/v3/users/followers/#list-users-followed-by-another-user
octokit.users.listFollowingForAuthenticatedUser();

// https://developer.github.com/v3/users/followers/#check-if-you-are-following-a-user
octokit.users.checkFollowing({ username });

// https://developer.github.com/v3/users/followers/#follow-a-user
octokit.users.follow({ username });

// https://developer.github.com/v3/users/followers/#unfollow-a-user
octokit.users.unfollow({ username });

// https://developer.github.com/v3/users/gpg_keys/#list-your-gpg-keys
octokit.users.listGpgKeys();

// https://developer.github.com/v3/users/gpg_keys/#create-a-gpg-key
octokit.users.createGpgKey({ armored_public_key });

// https://developer.github.com/v3/users/gpg_keys/#get-a-single-gpg-key
octokit.users.getGpgKey({ gpg_key_id });

// https://developer.github.com/v3/users/gpg_keys/#delete-a-gpg-key
octokit.users.deleteGpgKey({ gpg_key_id });

// https://developer.github.com/v3/apps/installations/#list-installations-for-a-user
octokit.apps.listInstallationsForAuthenticatedUser();

// https://developer.github.com/v3/apps/installations/#list-repositories-accessible-to-the-user-for-an-installation
octokit.apps.listInstallationReposForAuthenticatedUser({ installation_id });

// https://developer.github.com/v3/apps/installations/#add-repository-to-installation
octokit.apps.addRepoToInstallation({ installation_id, repository_id });

// https://developer.github.com/v3/apps/installations/#remove-repository-from-installation
octokit.apps.removeRepoFromInstallation({ installation_id, repository_id });

// https://developer.github.com/v3/issues/#list-issues
octokit.issues.listForAuthenticatedUser({
  filter,
  state,
  labels,
  sort,
  direction,
  since
});

// https://developer.github.com/v3/users/keys/#list-your-public-keys
octokit.users.listPublicKeys();

// https://developer.github.com/v3/users/keys/#create-a-public-key
octokit.users.createPublicKey({ title, key });

// https://developer.github.com/v3/users/keys/#get-a-single-public-key
octokit.users.getPublicKey({ key_id });

// https://developer.github.com/v3/users/keys/#delete-a-public-key
octokit.users.deletePublicKey({ key_id });

// https://developer.github.com/v3/apps/marketplace/#get-a-users-marketplace-purchases
octokit.apps.listMarketplacePurchasesForAuthenticatedUser();

// https://developer.github.com/v3/apps/marketplace/#get-a-users-marketplace-purchases
octokit.apps.listMarketplacePurchasesForAuthenticatedUserStubbed();

// https://developer.github.com/v3/orgs/members/#list-your-organization-memberships
octokit.orgs.listMemberships({ state });

// https://developer.github.com/v3/orgs/members/#get-your-organization-membership
octokit.orgs.getMembershipForAuthenticatedUser({ org });

// https://developer.github.com/v3/orgs/members/#edit-your-organization-membership
octokit.orgs.updateMembership({ org, state });

// https://developer.github.com/v3/migrations/users/#start-a-user-migration
octokit.migrations.startForAuthenticatedUser({
  repositories,
  lock_repositories,
  exclude_attachments
});

// https://developer.github.com/v3/migrations/users/#list-user-migrations
octokit.migrations.listForAuthenticatedUser();

// https://developer.github.com/v3/migrations/users/#get-the-status-of-a-user-migration
octokit.migrations.getStatusForAuthenticatedUser({ migration_id });

// https://developer.github.com/v3/migrations/users/#download-a-user-migration-archive
octokit.migrations.getArchiveForAuthenticatedUser({ migration_id });

// https://developer.github.com/v3/migrations/users/#delete-a-user-migration-archive
octokit.migrations.deleteArchiveForAuthenticatedUser({ migration_id });

// https://developer.github.com/v3/migrations/users/#unlock-a-user-repository
octokit.migrations.unlockRepoForAuthenticatedUser({ migration_id, repo_name });

// https://developer.github.com/v3/orgs/#list-your-organizations
octokit.orgs.listForAuthenticatedUser();

// https://developer.github.com/v3/projects/#create-a-user-project
octokit.projects.createForAuthenticatedUser({ name, body });

// https://developer.github.com/v3/users/emails/#list-public-email-addresses-for-a-user
octokit.users.listPublicEmails();

// https://developer.github.com/v3/repos/#list-your-repositories
octokit.repos.list({ visibility, affiliation, type, sort, direction });

// https://developer.github.com/v3/repos/#create
octokit.repos.createForAuthenticatedUser({
  name,
  description,
  homepage,
  private,
  visibility,
  has_issues,
  has_projects,
  has_wiki,
  is_template,
  team_id,
  auto_init,
  gitignore_template,
  license_template,
  allow_squash_merge,
  allow_merge_commit,
  allow_rebase_merge,
  delete_branch_on_merge
});

// https://developer.github.com/v3/repos/invitations/#list-a-users-repository-invitations
octokit.repos.listInvitationsForAuthenticatedUser();

// https://developer.github.com/v3/repos/invitations/#accept-a-repository-invitation
octokit.repos.acceptInvitation({ invitation_id });

// https://developer.github.com/v3/repos/invitations/#decline-a-repository-invitation
octokit.repos.declineInvitation({ invitation_id });

// https://developer.github.com/v3/activity/starring/#list-repositories-being-starred
octokit.activity.listReposStarredByAuthenticatedUser({ sort, direction });

// https://developer.github.com/v3/activity/starring/#check-if-you-are-starring-a-repository
octokit.activity.checkStarringRepo({ owner, repo });

// https://developer.github.com/v3/activity/starring/#star-a-repository
octokit.activity.starRepo({ owner, repo });

// https://developer.github.com/v3/activity/starring/#unstar-a-repository
octokit.activity.unstarRepo({ owner, repo });

// https://developer.github.com/v3/activity/watching/#list-repositories-being-watched
octokit.activity.listWatchedReposForAuthenticatedUser();

// https://developer.github.com/v3/teams/#list-user-teams
octokit.teams.listForAuthenticatedUser();

// https://developer.github.com/v3/migrations/users/#list-repositories-for-a-user-migration
octokit.migrations.listReposForUser({ migration_id });

// https://developer.github.com/v3/users/#get-all-users
octokit.users.list({ since });

// https://developer.github.com/v3/users/#get-a-single-user
octokit.users.getByUsername({ username });

// https://developer.github.com/v3/activity/events/#list-events-performed-by-a-user
octokit.activity.listEventsForUser({ username });

// https://developer.github.com/v3/activity/events/#list-events-for-an-organization
octokit.activity.listEventsForOrg({ username, org });

// https://developer.github.com/v3/activity/events/#list-public-events-performed-by-a-user
octokit.activity.listPublicEventsForUser({ username });

// https://developer.github.com/v3/users/followers/#list-followers-of-a-user
octokit.users.listFollowersForUser({ username });

// https://developer.github.com/v3/users/followers/#list-users-followed-by-another-user
octokit.users.listFollowingForUser({ username });

// https://developer.github.com/v3/users/followers/#check-if-one-user-follows-another
octokit.users.checkFollowingForUser({ username, target_user });

// https://developer.github.com/v3/gists/#list-a-users-gists
octokit.gists.listPublicForUser({ username, since });

// https://developer.github.com/v3/users/gpg_keys/#list-gpg-keys-for-a-user
octokit.users.listGpgKeysForUser({ username });

// https://developer.github.com/v3/users/#get-contextual-information-about-a-user
octokit.users.getContextForUser({ username, subject_type, subject_id });

// https://developer.github.com/v3/apps/#get-a-user-installation
octokit.apps.getUserInstallation({ username });

// DEPRECATED: octokit.apps.findUserInstallation() has been renamed to octokit.apps.getUserInstallation()
octokit.apps.findUserInstallation({ username });

// https://developer.github.com/v3/users/keys/#list-public-keys-for-a-user
octokit.users.listPublicKeysForUser({ username });

// https://developer.github.com/v3/orgs/#list-user-organizations
octokit.orgs.listForUser({ username });

// https://developer.github.com/v3/projects/#list-user-projects
octokit.projects.listForUser({ username, state });

// https://developer.github.com/v3/activity/events/#list-events-that-a-user-has-received
octokit.activity.listReceivedEventsForUser({ username });

// https://developer.github.com/v3/activity/events/#list-public-events-that-a-user-has-received
octokit.activity.listReceivedPublicEventsForUser({ username });

// https://developer.github.com/v3/repos/#list-user-repositories
octokit.repos.listForUser({ username, type, sort, direction });

// https://developer.github.com/v3/activity/starring/#list-repositories-being-starred
octokit.activity.listReposStarredByUser({ username, sort, direction });

// https://developer.github.com/v3/activity/watching/#list-repositories-being-watched
octokit.activity.listReposWatchedByUser({ username });

// https://developer.github.com/v3/repos/commits/#get-a-single-commit
octokit.repos.getCommitRefSha({ owner, ref, repo });

// https://developer.github.com/v3/git/refs/#get-all-references
octokit.git.listRefs({ owner, repo, namespace });

// https://developer.github.com/v3/issues/labels/#update-a-label
octokit.issues.updateLabel({
  owner,
  repo,
  current_name,
  color,
  name,
  description
});

// https://developer.github.com/v3/pulls/#create-a-pull-request
octokit.pulls.createFromIssue({
  owner,
  repo,
  base,
  draft,
  head,
  issue,
  maintainer_can_modify,
  owner,
  repo
});

// https://developer.github.com/v3/repos/releases/#upload-a-release-asset
octokit.repos.uploadReleaseAsset({ data, headers, label, name, url });
```

There is one method for each REST API endpoint documented at [https://developer.github.com/v3](https://developer.github.com/v3).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md)

## License

[MIT](LICENSE)
