declare const _default: {
    actions: {
        cancelWorkflowRun: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                run_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createOrUpdateSecretForRepo: {
            method: string;
            params: {
                encrypted_value: {
                    type: string;
                };
                key_id: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createRegistrationToken: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createRemoveToken: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteArtifact: {
            method: string;
            params: {
                artifact_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteSecretFromRepo: {
            method: string;
            params: {
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        downloadArtifact: {
            method: string;
            params: {
                archive_format: {
                    required: boolean;
                    type: string;
                };
                artifact_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getArtifact: {
            method: string;
            params: {
                artifact_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getPublicKey: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getSecret: {
            method: string;
            params: {
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getSelfHostedRunner: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                runner_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getWorkflow: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                workflow_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getWorkflowJob: {
            method: string;
            params: {
                job_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getWorkflowRun: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                run_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDownloadsForSelfHostedRunnerApplication: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listJobsForWorkflowRun: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                run_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listRepoWorkflowRuns: {
            method: string;
            params: {
                actor: {
                    type: string;
                };
                branch: {
                    type: string;
                };
                event: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                status: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listRepoWorkflows: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listSecretsForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listSelfHostedRunnersForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listWorkflowJobLogs: {
            method: string;
            params: {
                job_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listWorkflowRunArtifacts: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                run_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listWorkflowRunLogs: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                run_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listWorkflowRuns: {
            method: string;
            params: {
                actor: {
                    type: string;
                };
                branch: {
                    type: string;
                };
                event: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                status: {
                    enum: string[];
                    type: string;
                };
                workflow_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        reRunWorkflow: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                run_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeSelfHostedRunner: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                runner_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    activity: {
        checkStarringRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteRepoSubscription: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteThreadSubscription: {
            method: string;
            params: {
                thread_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRepoSubscription: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getThread: {
            method: string;
            params: {
                thread_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getThreadSubscription: {
            method: string;
            params: {
                thread_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listEventsForOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listEventsForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listFeeds: {
            method: string;
            params: {};
            url: string;
        };
        listNotifications: {
            method: string;
            params: {
                all: {
                    type: string;
                };
                before: {
                    type: string;
                };
                page: {
                    type: string;
                };
                participating: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listNotificationsForRepo: {
            method: string;
            params: {
                all: {
                    type: string;
                };
                before: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                participating: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicEvents: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicEventsForOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicEventsForRepoNetwork: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPublicEventsForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReceivedEventsForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReceivedPublicEventsForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listRepoEvents: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReposStarredByAuthenticatedUser: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listReposStarredByUser: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReposWatchedByUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listStargazersForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listWatchedReposForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listWatchersForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        markAsRead: {
            method: string;
            params: {
                last_read_at: {
                    type: string;
                };
            };
            url: string;
        };
        markNotificationsAsReadForRepo: {
            method: string;
            params: {
                last_read_at: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        markThreadAsRead: {
            method: string;
            params: {
                thread_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        setRepoSubscription: {
            method: string;
            params: {
                ignored: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                subscribed: {
                    type: string;
                };
            };
            url: string;
        };
        setThreadSubscription: {
            method: string;
            params: {
                ignored: {
                    type: string;
                };
                thread_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        starRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unstarRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    apps: {
        addRepoToInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                installation_id: {
                    required: boolean;
                    type: string;
                };
                repository_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkAccountIsAssociatedWithAny: {
            method: string;
            params: {
                account_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkAccountIsAssociatedWithAnyStubbed: {
            method: string;
            params: {
                account_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkAuthorization: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkToken: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                access_token: {
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createContentAttachment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                content_reference_id: {
                    required: boolean;
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createFromManifest: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                code: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createInstallationToken: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                installation_id: {
                    required: boolean;
                    type: string;
                };
                permissions: {
                    type: string;
                };
                repository_ids: {
                    type: string;
                };
            };
            url: string;
        };
        deleteAuthorization: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                access_token: {
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                installation_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteToken: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                access_token: {
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        findOrgInstallation: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        findRepoInstallation: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        findUserInstallation: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getAuthenticated: {
            headers: {
                accept: string;
            };
            method: string;
            params: {};
            url: string;
        };
        getBySlug: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                app_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                installation_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getOrgInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRepoInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getUserInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listAccountsUserOrOrgOnPlan: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                plan_id: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listAccountsUserOrOrgOnPlanStubbed: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                plan_id: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listInstallationReposForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                installation_id: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listInstallations: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listInstallationsForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listMarketplacePurchasesForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listMarketplacePurchasesForAuthenticatedUserStubbed: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPlans: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPlansStubbed: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listRepos: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        removeRepoFromInstallation: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                installation_id: {
                    required: boolean;
                    type: string;
                };
                repository_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        resetAuthorization: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        resetToken: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                access_token: {
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        revokeAuthorizationForApplication: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        revokeGrantForApplication: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        revokeInstallationToken: {
            headers: {
                accept: string;
            };
            method: string;
            params: {};
            url: string;
        };
    };
    checks: {
        create: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                actions: {
                    type: string;
                };
                "actions[].description": {
                    required: boolean;
                    type: string;
                };
                "actions[].identifier": {
                    required: boolean;
                    type: string;
                };
                "actions[].label": {
                    required: boolean;
                    type: string;
                };
                completed_at: {
                    type: string;
                };
                conclusion: {
                    enum: string[];
                    type: string;
                };
                details_url: {
                    type: string;
                };
                external_id: {
                    type: string;
                };
                head_sha: {
                    required: boolean;
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                output: {
                    type: string;
                };
                "output.annotations": {
                    type: string;
                };
                "output.annotations[].annotation_level": {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                "output.annotations[].end_column": {
                    type: string;
                };
                "output.annotations[].end_line": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].message": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].path": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].raw_details": {
                    type: string;
                };
                "output.annotations[].start_column": {
                    type: string;
                };
                "output.annotations[].start_line": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].title": {
                    type: string;
                };
                "output.images": {
                    type: string;
                };
                "output.images[].alt": {
                    required: boolean;
                    type: string;
                };
                "output.images[].caption": {
                    type: string;
                };
                "output.images[].image_url": {
                    required: boolean;
                    type: string;
                };
                "output.summary": {
                    required: boolean;
                    type: string;
                };
                "output.text": {
                    type: string;
                };
                "output.title": {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                started_at: {
                    type: string;
                };
                status: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        createSuite: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                head_sha: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                check_run_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getSuite: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                check_suite_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listAnnotations: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                check_run_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForRef: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                check_name: {
                    type: string;
                };
                filter: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                status: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listForSuite: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                check_name: {
                    type: string;
                };
                check_suite_id: {
                    required: boolean;
                    type: string;
                };
                filter: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                status: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listSuitesForRef: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                app_id: {
                    type: string;
                };
                check_name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        rerequestSuite: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                check_suite_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        setSuitesPreferences: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                auto_trigger_checks: {
                    type: string;
                };
                "auto_trigger_checks[].app_id": {
                    required: boolean;
                    type: string;
                };
                "auto_trigger_checks[].setting": {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                actions: {
                    type: string;
                };
                "actions[].description": {
                    required: boolean;
                    type: string;
                };
                "actions[].identifier": {
                    required: boolean;
                    type: string;
                };
                "actions[].label": {
                    required: boolean;
                    type: string;
                };
                check_run_id: {
                    required: boolean;
                    type: string;
                };
                completed_at: {
                    type: string;
                };
                conclusion: {
                    enum: string[];
                    type: string;
                };
                details_url: {
                    type: string;
                };
                external_id: {
                    type: string;
                };
                name: {
                    type: string;
                };
                output: {
                    type: string;
                };
                "output.annotations": {
                    type: string;
                };
                "output.annotations[].annotation_level": {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                "output.annotations[].end_column": {
                    type: string;
                };
                "output.annotations[].end_line": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].message": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].path": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].raw_details": {
                    type: string;
                };
                "output.annotations[].start_column": {
                    type: string;
                };
                "output.annotations[].start_line": {
                    required: boolean;
                    type: string;
                };
                "output.annotations[].title": {
                    type: string;
                };
                "output.images": {
                    type: string;
                };
                "output.images[].alt": {
                    required: boolean;
                    type: string;
                };
                "output.images[].caption": {
                    type: string;
                };
                "output.images[].image_url": {
                    required: boolean;
                    type: string;
                };
                "output.summary": {
                    required: boolean;
                    type: string;
                };
                "output.text": {
                    type: string;
                };
                "output.title": {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                started_at: {
                    type: string;
                };
                status: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
    };
    codesOfConduct: {
        getConductCode: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                key: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getForRepo: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listConductCodes: {
            headers: {
                accept: string;
            };
            method: string;
            params: {};
            url: string;
        };
    };
    emojis: {
        get: {
            method: string;
            params: {};
            url: string;
        };
    };
    gists: {
        checkIsStarred: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        create: {
            method: string;
            params: {
                description: {
                    type: string;
                };
                files: {
                    required: boolean;
                    type: string;
                };
                "files.content": {
                    type: string;
                };
                public: {
                    type: string;
                };
            };
            url: string;
        };
        createComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        delete: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        fork: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRevision: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listComments: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listCommits: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listForks: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPublic: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listStarred: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        star: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unstar: {
            method: string;
            params: {
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            method: string;
            params: {
                description: {
                    type: string;
                };
                files: {
                    type: string;
                };
                "files.content": {
                    type: string;
                };
                "files.filename": {
                    type: string;
                };
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_id: {
                    required: boolean;
                    type: string;
                };
                gist_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    git: {
        createBlob: {
            method: string;
            params: {
                content: {
                    required: boolean;
                    type: string;
                };
                encoding: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createCommit: {
            method: string;
            params: {
                author: {
                    type: string;
                };
                "author.date": {
                    type: string;
                };
                "author.email": {
                    type: string;
                };
                "author.name": {
                    type: string;
                };
                committer: {
                    type: string;
                };
                "committer.date": {
                    type: string;
                };
                "committer.email": {
                    type: string;
                };
                "committer.name": {
                    type: string;
                };
                message: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                parents: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                signature: {
                    type: string;
                };
                tree: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createRef: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createTag: {
            method: string;
            params: {
                message: {
                    required: boolean;
                    type: string;
                };
                object: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tag: {
                    required: boolean;
                    type: string;
                };
                tagger: {
                    type: string;
                };
                "tagger.date": {
                    type: string;
                };
                "tagger.email": {
                    type: string;
                };
                "tagger.name": {
                    type: string;
                };
                type: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createTree: {
            method: string;
            params: {
                base_tree: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tree: {
                    required: boolean;
                    type: string;
                };
                "tree[].content": {
                    type: string;
                };
                "tree[].mode": {
                    enum: string[];
                    type: string;
                };
                "tree[].path": {
                    type: string;
                };
                "tree[].sha": {
                    allowNull: boolean;
                    type: string;
                };
                "tree[].type": {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        deleteRef: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getBlob: {
            method: string;
            params: {
                file_sha: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommit: {
            method: string;
            params: {
                commit_sha: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRef: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getTag: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tag_sha: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getTree: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                recursive: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tree_sha: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listMatchingRefs: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listRefs: {
            method: string;
            params: {
                namespace: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateRef: {
            method: string;
            params: {
                force: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    gitignore: {
        getTemplate: {
            method: string;
            params: {
                name: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listTemplates: {
            method: string;
            params: {};
            url: string;
        };
    };
    interactions: {
        addOrUpdateRestrictionsForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                limit: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateRestrictionsForRepo: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                limit: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRestrictionsForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRestrictionsForRepo: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeRestrictionsForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeRestrictionsForRepo: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    issues: {
        addAssignees: {
            method: string;
            params: {
                assignees: {
                    type: string;
                };
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addLabels: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                labels: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkAssignee: {
            method: string;
            params: {
                assignee: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        create: {
            method: string;
            params: {
                assignee: {
                    type: string;
                };
                assignees: {
                    type: string;
                };
                body: {
                    type: string;
                };
                labels: {
                    type: string;
                };
                milestone: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createLabel: {
            method: string;
            params: {
                color: {
                    required: boolean;
                    type: string;
                };
                description: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createMilestone: {
            method: string;
            params: {
                description: {
                    type: string;
                };
                due_on: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteLabel: {
            method: string;
            params: {
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteMilestone: {
            method: string;
            params: {
                milestone_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getEvent: {
            method: string;
            params: {
                event_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getLabel: {
            method: string;
            params: {
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMilestone: {
            method: string;
            params: {
                milestone_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                filter: {
                    enum: string[];
                    type: string;
                };
                labels: {
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listAssignees: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listComments: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listCommentsForRepo: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listEvents: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listEventsForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listEventsForTimeline: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForAuthenticatedUser: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                filter: {
                    enum: string[];
                    type: string;
                };
                labels: {
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listForOrg: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                filter: {
                    enum: string[];
                    type: string;
                };
                labels: {
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listForRepo: {
            method: string;
            params: {
                assignee: {
                    type: string;
                };
                creator: {
                    type: string;
                };
                direction: {
                    enum: string[];
                    type: string;
                };
                labels: {
                    type: string;
                };
                mentioned: {
                    type: string;
                };
                milestone: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listLabelsForMilestone: {
            method: string;
            params: {
                milestone_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listLabelsForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listLabelsOnIssue: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listMilestonesForRepo: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        lock: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                lock_reason: {
                    enum: string[];
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeAssignees: {
            method: string;
            params: {
                assignees: {
                    type: string;
                };
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeLabel: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeLabels: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        replaceLabels: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                labels: {
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unlock: {
            method: string;
            params: {
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            method: string;
            params: {
                assignee: {
                    type: string;
                };
                assignees: {
                    type: string;
                };
                body: {
                    type: string;
                };
                issue_number: {
                    required: boolean;
                    type: string;
                };
                labels: {
                    type: string;
                };
                milestone: {
                    allowNull: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        updateComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateLabel: {
            method: string;
            params: {
                color: {
                    type: string;
                };
                current_name: {
                    required: boolean;
                    type: string;
                };
                description: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateMilestone: {
            method: string;
            params: {
                description: {
                    type: string;
                };
                due_on: {
                    type: string;
                };
                milestone_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
    };
    licenses: {
        get: {
            method: string;
            params: {
                license: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getForRepo: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            deprecated: string;
            method: string;
            params: {};
            url: string;
        };
        listCommonlyUsed: {
            method: string;
            params: {};
            url: string;
        };
    };
    markdown: {
        render: {
            method: string;
            params: {
                context: {
                    type: string;
                };
                mode: {
                    enum: string[];
                    type: string;
                };
                text: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        renderRaw: {
            headers: {
                "content-type": string;
            };
            method: string;
            params: {
                data: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    meta: {
        get: {
            method: string;
            params: {};
            url: string;
        };
    };
    migrations: {
        cancelImport: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteArchiveForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteArchiveForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        downloadArchiveForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getArchiveForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getArchiveForOrg: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommitAuthors: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        getImportProgress: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getLargeFiles: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getStatusForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getStatusForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listReposForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listReposForUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        mapCommitAuthor: {
            method: string;
            params: {
                author_id: {
                    required: boolean;
                    type: string;
                };
                email: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        setLfsPreference: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                use_lfs: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        startForAuthenticatedUser: {
            method: string;
            params: {
                exclude_attachments: {
                    type: string;
                };
                lock_repositories: {
                    type: string;
                };
                repositories: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        startForOrg: {
            method: string;
            params: {
                exclude_attachments: {
                    type: string;
                };
                lock_repositories: {
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                repositories: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        startImport: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tfvc_project: {
                    type: string;
                };
                vcs: {
                    enum: string[];
                    type: string;
                };
                vcs_password: {
                    type: string;
                };
                vcs_url: {
                    required: boolean;
                    type: string;
                };
                vcs_username: {
                    type: string;
                };
            };
            url: string;
        };
        unlockRepoForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                repo_name: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unlockRepoForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                migration_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                repo_name: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateImport: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                vcs_password: {
                    type: string;
                };
                vcs_username: {
                    type: string;
                };
            };
            url: string;
        };
    };
    oauthAuthorizations: {
        checkAuthorization: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createAuthorization: {
            deprecated: string;
            method: string;
            params: {
                client_id: {
                    type: string;
                };
                client_secret: {
                    type: string;
                };
                fingerprint: {
                    type: string;
                };
                note: {
                    required: boolean;
                    type: string;
                };
                note_url: {
                    type: string;
                };
                scopes: {
                    type: string;
                };
            };
            url: string;
        };
        deleteAuthorization: {
            deprecated: string;
            method: string;
            params: {
                authorization_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteGrant: {
            deprecated: string;
            method: string;
            params: {
                grant_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getAuthorization: {
            deprecated: string;
            method: string;
            params: {
                authorization_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getGrant: {
            deprecated: string;
            method: string;
            params: {
                grant_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getOrCreateAuthorizationForApp: {
            deprecated: string;
            method: string;
            params: {
                client_id: {
                    required: boolean;
                    type: string;
                };
                client_secret: {
                    required: boolean;
                    type: string;
                };
                fingerprint: {
                    type: string;
                };
                note: {
                    type: string;
                };
                note_url: {
                    type: string;
                };
                scopes: {
                    type: string;
                };
            };
            url: string;
        };
        getOrCreateAuthorizationForAppAndFingerprint: {
            deprecated: string;
            method: string;
            params: {
                client_id: {
                    required: boolean;
                    type: string;
                };
                client_secret: {
                    required: boolean;
                    type: string;
                };
                fingerprint: {
                    required: boolean;
                    type: string;
                };
                note: {
                    type: string;
                };
                note_url: {
                    type: string;
                };
                scopes: {
                    type: string;
                };
            };
            url: string;
        };
        getOrCreateAuthorizationForAppFingerprint: {
            deprecated: string;
            method: string;
            params: {
                client_id: {
                    required: boolean;
                    type: string;
                };
                client_secret: {
                    required: boolean;
                    type: string;
                };
                fingerprint: {
                    required: boolean;
                    type: string;
                };
                note: {
                    type: string;
                };
                note_url: {
                    type: string;
                };
                scopes: {
                    type: string;
                };
            };
            url: string;
        };
        listAuthorizations: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listGrants: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        resetAuthorization: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        revokeAuthorizationForApplication: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        revokeGrantForApplication: {
            deprecated: string;
            method: string;
            params: {
                access_token: {
                    required: boolean;
                    type: string;
                };
                client_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateAuthorization: {
            deprecated: string;
            method: string;
            params: {
                add_scopes: {
                    type: string;
                };
                authorization_id: {
                    required: boolean;
                    type: string;
                };
                fingerprint: {
                    type: string;
                };
                note: {
                    type: string;
                };
                note_url: {
                    type: string;
                };
                remove_scopes: {
                    type: string;
                };
                scopes: {
                    type: string;
                };
            };
            url: string;
        };
    };
    orgs: {
        addOrUpdateMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        blockUser: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkBlockedUser: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkPublicMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        concealMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        convertMemberToOutsideCollaborator: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createHook: {
            method: string;
            params: {
                active: {
                    type: string;
                };
                config: {
                    required: boolean;
                    type: string;
                };
                "config.content_type": {
                    type: string;
                };
                "config.insecure_ssl": {
                    type: string;
                };
                "config.secret": {
                    type: string;
                };
                "config.url": {
                    required: boolean;
                    type: string;
                };
                events: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createInvitation: {
            method: string;
            params: {
                email: {
                    type: string;
                };
                invitee_id: {
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
                team_ids: {
                    type: string;
                };
            };
            url: string;
        };
        deleteHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMembershipForAuthenticatedUser: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listBlockedUsers: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listHooks: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listInstallations: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listInvitationTeams: {
            method: string;
            params: {
                invitation_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listMembers: {
            method: string;
            params: {
                filter: {
                    enum: string[];
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listMemberships: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listOutsideCollaborators: {
            method: string;
            params: {
                filter: {
                    enum: string[];
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPendingInvitations: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicMembers: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        pingHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        publicizeMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMember: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeOutsideCollaborator: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unblockUser: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            method: string;
            params: {
                billing_email: {
                    type: string;
                };
                company: {
                    type: string;
                };
                default_repository_permission: {
                    enum: string[];
                    type: string;
                };
                description: {
                    type: string;
                };
                email: {
                    type: string;
                };
                has_organization_projects: {
                    type: string;
                };
                has_repository_projects: {
                    type: string;
                };
                location: {
                    type: string;
                };
                members_allowed_repository_creation_type: {
                    enum: string[];
                    type: string;
                };
                members_can_create_internal_repositories: {
                    type: string;
                };
                members_can_create_private_repositories: {
                    type: string;
                };
                members_can_create_public_repositories: {
                    type: string;
                };
                members_can_create_repositories: {
                    type: string;
                };
                name: {
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateHook: {
            method: string;
            params: {
                active: {
                    type: string;
                };
                config: {
                    type: string;
                };
                "config.content_type": {
                    type: string;
                };
                "config.insecure_ssl": {
                    type: string;
                };
                "config.secret": {
                    type: string;
                };
                "config.url": {
                    required: boolean;
                    type: string;
                };
                events: {
                    type: string;
                };
                hook_id: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateMembership: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    projects: {
        addCollaborator: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                permission: {
                    enum: string[];
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createCard: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                column_id: {
                    required: boolean;
                    type: string;
                };
                content_id: {
                    type: string;
                };
                content_type: {
                    type: string;
                };
                note: {
                    type: string;
                };
            };
            url: string;
        };
        createColumn: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                name: {
                    required: boolean;
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForAuthenticatedUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                body: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                body: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForRepo: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                body: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        delete: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteCard: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                card_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteColumn: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                column_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCard: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                card_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getColumn: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                column_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listCards: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                archived_state: {
                    enum: string[];
                    type: string;
                };
                column_id: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listCollaborators: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                affiliation: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listColumns: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listForRepo: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listForUser: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        moveCard: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                card_id: {
                    required: boolean;
                    type: string;
                };
                column_id: {
                    type: string;
                };
                position: {
                    required: boolean;
                    type: string;
                    validation: string;
                };
            };
            url: string;
        };
        moveColumn: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                column_id: {
                    required: boolean;
                    type: string;
                };
                position: {
                    required: boolean;
                    type: string;
                    validation: string;
                };
            };
            url: string;
        };
        removeCollaborator: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        reviewUserPermissionLevel: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                body: {
                    type: string;
                };
                name: {
                    type: string;
                };
                organization_permission: {
                    type: string;
                };
                private: {
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        updateCard: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                archived: {
                    type: string;
                };
                card_id: {
                    required: boolean;
                    type: string;
                };
                note: {
                    type: string;
                };
            };
            url: string;
        };
        updateColumn: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                column_id: {
                    required: boolean;
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    pulls: {
        checkIfMerged: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        create: {
            method: string;
            params: {
                base: {
                    required: boolean;
                    type: string;
                };
                body: {
                    type: string;
                };
                draft: {
                    type: string;
                };
                head: {
                    required: boolean;
                    type: string;
                };
                maintainer_can_modify: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                commit_id: {
                    required: boolean;
                    type: string;
                };
                in_reply_to: {
                    deprecated: boolean;
                    description: string;
                    type: string;
                };
                line: {
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                position: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                side: {
                    enum: string[];
                    type: string;
                };
                start_line: {
                    type: string;
                };
                start_side: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        createCommentReply: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                commit_id: {
                    required: boolean;
                    type: string;
                };
                in_reply_to: {
                    deprecated: boolean;
                    description: string;
                    type: string;
                };
                line: {
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                position: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                side: {
                    enum: string[];
                    type: string;
                };
                start_line: {
                    type: string;
                };
                start_side: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        createFromIssue: {
            deprecated: string;
            method: string;
            params: {
                base: {
                    required: boolean;
                    type: string;
                };
                draft: {
                    type: string;
                };
                head: {
                    required: boolean;
                    type: string;
                };
                issue: {
                    required: boolean;
                    type: string;
                };
                maintainer_can_modify: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createReview: {
            method: string;
            params: {
                body: {
                    type: string;
                };
                comments: {
                    type: string;
                };
                "comments[].body": {
                    required: boolean;
                    type: string;
                };
                "comments[].path": {
                    required: boolean;
                    type: string;
                };
                "comments[].position": {
                    required: boolean;
                    type: string;
                };
                commit_id: {
                    type: string;
                };
                event: {
                    enum: string[];
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createReviewCommentReply: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createReviewRequest: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                reviewers: {
                    type: string;
                };
                team_reviewers: {
                    type: string;
                };
            };
            url: string;
        };
        deleteComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deletePendingReview: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                review_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteReviewRequest: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                reviewers: {
                    type: string;
                };
                team_reviewers: {
                    type: string;
                };
            };
            url: string;
        };
        dismissReview: {
            method: string;
            params: {
                message: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                review_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommentsForReview: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                review_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getReview: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                review_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                base: {
                    type: string;
                };
                direction: {
                    enum: string[];
                    type: string;
                };
                head: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listComments: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listCommentsForRepo: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                since: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listCommits: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listFiles: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReviewRequests: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReviews: {
            method: string;
            params: {
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        merge: {
            method: string;
            params: {
                commit_message: {
                    type: string;
                };
                commit_title: {
                    type: string;
                };
                merge_method: {
                    enum: string[];
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    type: string;
                };
            };
            url: string;
        };
        submitReview: {
            method: string;
            params: {
                body: {
                    type: string;
                };
                event: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                review_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            method: string;
            params: {
                base: {
                    type: string;
                };
                body: {
                    type: string;
                };
                maintainer_can_modify: {
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        updateBranch: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                expected_head_sha: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateReview: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                pull_number: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                review_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    rateLimit: {
        get: {
            method: string;
            params: {};
            url: string;
        };
    };
    reactions: {
        createForCommitComment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForIssue: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForIssueComment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForPullRequestReviewComment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForTeamDiscussion: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForTeamDiscussionComment: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForTeamDiscussionCommentInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForTeamDiscussionCommentLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForTeamDiscussionInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createForTeamDiscussionLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        delete: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                reaction_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForCommitComment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForIssue: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    type: string;
                };
                issue_number: {
                    required: boolean;
                    type: string;
                };
                number: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForIssueComment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForPullRequestReviewComment: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForTeamDiscussion: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForTeamDiscussionComment: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForTeamDiscussionCommentInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForTeamDiscussionCommentLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                content: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForTeamDiscussionInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForTeamDiscussionLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                content: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    repos: {
        acceptInvitation: {
            method: string;
            params: {
                invitation_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addCollaborator: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addDeployKey: {
            method: string;
            params: {
                key: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                read_only: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        addProtectedBranchAdminEnforcement: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addProtectedBranchAppRestrictions: {
            method: string;
            params: {
                apps: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addProtectedBranchRequiredSignatures: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addProtectedBranchRequiredStatusChecksContexts: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                contexts: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addProtectedBranchTeamRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                teams: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addProtectedBranchUserRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                users: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkCollaborator: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkVulnerabilityAlerts: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        compareCommits: {
            method: string;
            params: {
                base: {
                    required: boolean;
                    type: string;
                };
                head: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createCommitComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                commit_sha: {
                    required: boolean;
                    type: string;
                };
                line: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    type: string;
                };
                position: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createDeployment: {
            method: string;
            params: {
                auto_merge: {
                    type: string;
                };
                description: {
                    type: string;
                };
                environment: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                payload: {
                    type: string;
                };
                production_environment: {
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                required_contexts: {
                    type: string;
                };
                task: {
                    type: string;
                };
                transient_environment: {
                    type: string;
                };
            };
            url: string;
        };
        createDeploymentStatus: {
            method: string;
            params: {
                auto_inactive: {
                    type: string;
                };
                deployment_id: {
                    required: boolean;
                    type: string;
                };
                description: {
                    type: string;
                };
                environment: {
                    enum: string[];
                    type: string;
                };
                environment_url: {
                    type: string;
                };
                log_url: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                target_url: {
                    type: string;
                };
            };
            url: string;
        };
        createDispatchEvent: {
            method: string;
            params: {
                client_payload: {
                    type: string;
                };
                event_type: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createFile: {
            deprecated: string;
            method: string;
            params: {
                author: {
                    type: string;
                };
                "author.email": {
                    required: boolean;
                    type: string;
                };
                "author.name": {
                    required: boolean;
                    type: string;
                };
                branch: {
                    type: string;
                };
                committer: {
                    type: string;
                };
                "committer.email": {
                    required: boolean;
                    type: string;
                };
                "committer.name": {
                    required: boolean;
                    type: string;
                };
                content: {
                    required: boolean;
                    type: string;
                };
                message: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    type: string;
                };
            };
            url: string;
        };
        createForAuthenticatedUser: {
            method: string;
            params: {
                allow_merge_commit: {
                    type: string;
                };
                allow_rebase_merge: {
                    type: string;
                };
                allow_squash_merge: {
                    type: string;
                };
                auto_init: {
                    type: string;
                };
                delete_branch_on_merge: {
                    type: string;
                };
                description: {
                    type: string;
                };
                gitignore_template: {
                    type: string;
                };
                has_issues: {
                    type: string;
                };
                has_projects: {
                    type: string;
                };
                has_wiki: {
                    type: string;
                };
                homepage: {
                    type: string;
                };
                is_template: {
                    type: string;
                };
                license_template: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                private: {
                    type: string;
                };
                team_id: {
                    type: string;
                };
                visibility: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        createFork: {
            method: string;
            params: {
                organization: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createHook: {
            method: string;
            params: {
                active: {
                    type: string;
                };
                config: {
                    required: boolean;
                    type: string;
                };
                "config.content_type": {
                    type: string;
                };
                "config.insecure_ssl": {
                    type: string;
                };
                "config.secret": {
                    type: string;
                };
                "config.url": {
                    required: boolean;
                    type: string;
                };
                events: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createInOrg: {
            method: string;
            params: {
                allow_merge_commit: {
                    type: string;
                };
                allow_rebase_merge: {
                    type: string;
                };
                allow_squash_merge: {
                    type: string;
                };
                auto_init: {
                    type: string;
                };
                delete_branch_on_merge: {
                    type: string;
                };
                description: {
                    type: string;
                };
                gitignore_template: {
                    type: string;
                };
                has_issues: {
                    type: string;
                };
                has_projects: {
                    type: string;
                };
                has_wiki: {
                    type: string;
                };
                homepage: {
                    type: string;
                };
                is_template: {
                    type: string;
                };
                license_template: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                private: {
                    type: string;
                };
                team_id: {
                    type: string;
                };
                visibility: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        createOrUpdateFile: {
            method: string;
            params: {
                author: {
                    type: string;
                };
                "author.email": {
                    required: boolean;
                    type: string;
                };
                "author.name": {
                    required: boolean;
                    type: string;
                };
                branch: {
                    type: string;
                };
                committer: {
                    type: string;
                };
                "committer.email": {
                    required: boolean;
                    type: string;
                };
                "committer.name": {
                    required: boolean;
                    type: string;
                };
                content: {
                    required: boolean;
                    type: string;
                };
                message: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    type: string;
                };
            };
            url: string;
        };
        createRelease: {
            method: string;
            params: {
                body: {
                    type: string;
                };
                draft: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                prerelease: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tag_name: {
                    required: boolean;
                    type: string;
                };
                target_commitish: {
                    type: string;
                };
            };
            url: string;
        };
        createStatus: {
            method: string;
            params: {
                context: {
                    type: string;
                };
                description: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    required: boolean;
                    type: string;
                };
                state: {
                    enum: string[];
                    required: boolean;
                    type: string;
                };
                target_url: {
                    type: string;
                };
            };
            url: string;
        };
        createUsingTemplate: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                description: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    type: string;
                };
                private: {
                    type: string;
                };
                template_owner: {
                    required: boolean;
                    type: string;
                };
                template_repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        declineInvitation: {
            method: string;
            params: {
                invitation_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        delete: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteCommitComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDownload: {
            method: string;
            params: {
                download_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteFile: {
            method: string;
            params: {
                author: {
                    type: string;
                };
                "author.email": {
                    type: string;
                };
                "author.name": {
                    type: string;
                };
                branch: {
                    type: string;
                };
                committer: {
                    type: string;
                };
                "committer.email": {
                    type: string;
                };
                "committer.name": {
                    type: string;
                };
                message: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteInvitation: {
            method: string;
            params: {
                invitation_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteRelease: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                release_id: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteReleaseAsset: {
            method: string;
            params: {
                asset_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        disableAutomatedSecurityFixes: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        disablePagesSite: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        disableVulnerabilityAlerts: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        enableAutomatedSecurityFixes: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        enablePagesSite: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                source: {
                    type: string;
                };
                "source.branch": {
                    enum: string[];
                    type: string;
                };
                "source.path": {
                    type: string;
                };
            };
            url: string;
        };
        enableVulnerabilityAlerts: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getAppsWithAccessToProtectedBranch: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getArchiveLink: {
            method: string;
            params: {
                archive_format: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getBranch: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getBranchProtection: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getClones: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                per: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCodeFrequencyStats: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCollaboratorPermissionLevel: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCombinedStatusForRef: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommit: {
            method: string;
            params: {
                commit_sha: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommitActivityStats: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommitComment: {
            method: string;
            params: {
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getCommitRefSha: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getContents: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getContributorsStats: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDeployKey: {
            method: string;
            params: {
                key_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDeployment: {
            method: string;
            params: {
                deployment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDeploymentStatus: {
            method: string;
            params: {
                deployment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                status_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDownload: {
            method: string;
            params: {
                download_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getLatestPagesBuild: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getLatestRelease: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getPages: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getPagesBuild: {
            method: string;
            params: {
                build_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getParticipationStats: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getProtectedBranchAdminEnforcement: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getProtectedBranchPullRequestReviewEnforcement: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getProtectedBranchRequiredSignatures: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getProtectedBranchRequiredStatusChecks: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getProtectedBranchRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getPunchCardStats: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getReadme: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                ref: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getRelease: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                release_id: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getReleaseAsset: {
            method: string;
            params: {
                asset_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getReleaseByTag: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tag: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getTeamsWithAccessToProtectedBranch: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getTopPaths: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getTopReferrers: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getUsersWithAccessToProtectedBranch: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getViews: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                per: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                affiliation: {
                    type: string;
                };
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                type: {
                    enum: string[];
                    type: string;
                };
                visibility: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listAppsWithAccessToProtectedBranch: {
            deprecated: string;
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listAssetsForRelease: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                release_id: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listBranches: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                protected: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listBranchesForHeadCommit: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                commit_sha: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listCollaborators: {
            method: string;
            params: {
                affiliation: {
                    enum: string[];
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listCommentsForCommit: {
            method: string;
            params: {
                commit_sha: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                ref: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listCommitComments: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listCommits: {
            method: string;
            params: {
                author: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                path: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    type: string;
                };
                since: {
                    type: string;
                };
                until: {
                    type: string;
                };
            };
            url: string;
        };
        listContributors: {
            method: string;
            params: {
                anon: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDeployKeys: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDeploymentStatuses: {
            method: string;
            params: {
                deployment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDeployments: {
            method: string;
            params: {
                environment: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                ref: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    type: string;
                };
                task: {
                    type: string;
                };
            };
            url: string;
        };
        listDownloads: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForOrg: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                type: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listForUser: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
                type: {
                    enum: string[];
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForks: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        listHooks: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listInvitations: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listInvitationsForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listLanguages: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPagesBuilds: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listProtectedBranchRequiredStatusChecksContexts: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listProtectedBranchTeamRestrictions: {
            deprecated: string;
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listProtectedBranchUserRestrictions: {
            deprecated: string;
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPublic: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listPullRequestsAssociatedWithCommit: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                commit_sha: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReleases: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listStatusesForRef: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                ref: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listTags: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listTeams: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listTeamsWithAccessToProtectedBranch: {
            deprecated: string;
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listTopics: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listUsersWithAccessToProtectedBranch: {
            deprecated: string;
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        merge: {
            method: string;
            params: {
                base: {
                    required: boolean;
                    type: string;
                };
                commit_message: {
                    type: string;
                };
                head: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        pingHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeBranchProtection: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeCollaborator: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeDeployKey: {
            method: string;
            params: {
                key_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchAdminEnforcement: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchAppRestrictions: {
            method: string;
            params: {
                apps: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchPullRequestReviewEnforcement: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchRequiredSignatures: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchRequiredStatusChecks: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchRequiredStatusChecksContexts: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                contexts: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchTeamRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                teams: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProtectedBranchUserRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                users: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        replaceProtectedBranchAppRestrictions: {
            method: string;
            params: {
                apps: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        replaceProtectedBranchRequiredStatusChecksContexts: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                contexts: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        replaceProtectedBranchTeamRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                teams: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        replaceProtectedBranchUserRestrictions: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                users: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        replaceTopics: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                names: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        requestPageBuild: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        retrieveCommunityProfileMetrics: {
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        testPushHook: {
            method: string;
            params: {
                hook_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        transfer: {
            method: string;
            params: {
                new_owner: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_ids: {
                    type: string;
                };
            };
            url: string;
        };
        update: {
            method: string;
            params: {
                allow_merge_commit: {
                    type: string;
                };
                allow_rebase_merge: {
                    type: string;
                };
                allow_squash_merge: {
                    type: string;
                };
                archived: {
                    type: string;
                };
                default_branch: {
                    type: string;
                };
                delete_branch_on_merge: {
                    type: string;
                };
                description: {
                    type: string;
                };
                has_issues: {
                    type: string;
                };
                has_projects: {
                    type: string;
                };
                has_wiki: {
                    type: string;
                };
                homepage: {
                    type: string;
                };
                is_template: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                private: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                visibility: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        updateBranchProtection: {
            method: string;
            params: {
                allow_deletions: {
                    type: string;
                };
                allow_force_pushes: {
                    allowNull: boolean;
                    type: string;
                };
                branch: {
                    required: boolean;
                    type: string;
                };
                enforce_admins: {
                    allowNull: boolean;
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                required_linear_history: {
                    type: string;
                };
                required_pull_request_reviews: {
                    allowNull: boolean;
                    required: boolean;
                    type: string;
                };
                "required_pull_request_reviews.dismiss_stale_reviews": {
                    type: string;
                };
                "required_pull_request_reviews.dismissal_restrictions": {
                    type: string;
                };
                "required_pull_request_reviews.dismissal_restrictions.teams": {
                    type: string;
                };
                "required_pull_request_reviews.dismissal_restrictions.users": {
                    type: string;
                };
                "required_pull_request_reviews.require_code_owner_reviews": {
                    type: string;
                };
                "required_pull_request_reviews.required_approving_review_count": {
                    type: string;
                };
                required_status_checks: {
                    allowNull: boolean;
                    required: boolean;
                    type: string;
                };
                "required_status_checks.contexts": {
                    required: boolean;
                    type: string;
                };
                "required_status_checks.strict": {
                    required: boolean;
                    type: string;
                };
                restrictions: {
                    allowNull: boolean;
                    required: boolean;
                    type: string;
                };
                "restrictions.apps": {
                    type: string;
                };
                "restrictions.teams": {
                    required: boolean;
                    type: string;
                };
                "restrictions.users": {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateCommitComment: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateFile: {
            deprecated: string;
            method: string;
            params: {
                author: {
                    type: string;
                };
                "author.email": {
                    required: boolean;
                    type: string;
                };
                "author.name": {
                    required: boolean;
                    type: string;
                };
                branch: {
                    type: string;
                };
                committer: {
                    type: string;
                };
                "committer.email": {
                    required: boolean;
                    type: string;
                };
                "committer.name": {
                    required: boolean;
                    type: string;
                };
                content: {
                    required: boolean;
                    type: string;
                };
                message: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                path: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                sha: {
                    type: string;
                };
            };
            url: string;
        };
        updateHook: {
            method: string;
            params: {
                active: {
                    type: string;
                };
                add_events: {
                    type: string;
                };
                config: {
                    type: string;
                };
                "config.content_type": {
                    type: string;
                };
                "config.insecure_ssl": {
                    type: string;
                };
                "config.secret": {
                    type: string;
                };
                "config.url": {
                    required: boolean;
                    type: string;
                };
                events: {
                    type: string;
                };
                hook_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                remove_events: {
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateInformationAboutPagesSite: {
            method: string;
            params: {
                cname: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                source: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        updateInvitation: {
            method: string;
            params: {
                invitation_id: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                permissions: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateProtectedBranchPullRequestReviewEnforcement: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                dismiss_stale_reviews: {
                    type: string;
                };
                dismissal_restrictions: {
                    type: string;
                };
                "dismissal_restrictions.teams": {
                    type: string;
                };
                "dismissal_restrictions.users": {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                require_code_owner_reviews: {
                    type: string;
                };
                required_approving_review_count: {
                    type: string;
                };
            };
            url: string;
        };
        updateProtectedBranchRequiredStatusChecks: {
            method: string;
            params: {
                branch: {
                    required: boolean;
                    type: string;
                };
                contexts: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                strict: {
                    type: string;
                };
            };
            url: string;
        };
        updateRelease: {
            method: string;
            params: {
                body: {
                    type: string;
                };
                draft: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                prerelease: {
                    type: string;
                };
                release_id: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                tag_name: {
                    type: string;
                };
                target_commitish: {
                    type: string;
                };
            };
            url: string;
        };
        updateReleaseAsset: {
            method: string;
            params: {
                asset_id: {
                    required: boolean;
                    type: string;
                };
                label: {
                    type: string;
                };
                name: {
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        uploadReleaseAsset: {
            method: string;
            params: {
                data: {
                    mapTo: string;
                    required: boolean;
                    type: string;
                };
                file: {
                    alias: string;
                    deprecated: boolean;
                    type: string;
                };
                headers: {
                    required: boolean;
                    type: string;
                };
                "headers.content-length": {
                    required: boolean;
                    type: string;
                };
                "headers.content-type": {
                    required: boolean;
                    type: string;
                };
                label: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                url: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    search: {
        code: {
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        commits: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        issues: {
            deprecated: string;
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        issuesAndPullRequests: {
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        labels: {
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                repository_id: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        repos: {
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
        topics: {
            method: string;
            params: {
                q: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        users: {
            method: string;
            params: {
                order: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                q: {
                    required: boolean;
                    type: string;
                };
                sort: {
                    enum: string[];
                    type: string;
                };
            };
            url: string;
        };
    };
    teams: {
        addMember: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addMemberLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateMembership: {
            deprecated: string;
            method: string;
            params: {
                role: {
                    enum: string[];
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateMembershipInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateMembershipLegacy: {
            deprecated: string;
            method: string;
            params: {
                role: {
                    enum: string[];
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateProject: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                permission: {
                    enum: string[];
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateProjectInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateProjectLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                permission: {
                    enum: string[];
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateRepo: {
            deprecated: string;
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateRepoInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        addOrUpdateRepoLegacy: {
            deprecated: string;
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkManagesRepo: {
            deprecated: string;
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkManagesRepoInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkManagesRepoLegacy: {
            deprecated: string;
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        create: {
            method: string;
            params: {
                description: {
                    type: string;
                };
                maintainers: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                parent_team_id: {
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                privacy: {
                    enum: string[];
                    type: string;
                };
                repo_names: {
                    type: string;
                };
            };
            url: string;
        };
        createDiscussion: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                private: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createDiscussionComment: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createDiscussionCommentInOrg: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createDiscussionCommentLegacy: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createDiscussionInOrg: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                private: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createDiscussionLegacy: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                private: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
                title: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        delete: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDiscussion: {
            deprecated: string;
            method: string;
            params: {
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDiscussionComment: {
            deprecated: string;
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDiscussionCommentInOrg: {
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDiscussionCommentLegacy: {
            deprecated: string;
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDiscussionInOrg: {
            method: string;
            params: {
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteDiscussionLegacy: {
            deprecated: string;
            method: string;
            params: {
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        get: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getByName: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDiscussion: {
            deprecated: string;
            method: string;
            params: {
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDiscussionComment: {
            deprecated: string;
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDiscussionCommentInOrg: {
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDiscussionCommentLegacy: {
            deprecated: string;
            method: string;
            params: {
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDiscussionInOrg: {
            method: string;
            params: {
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getDiscussionLegacy: {
            deprecated: string;
            method: string;
            params: {
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMember: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMemberLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMembership: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMembershipInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getMembershipLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listChild: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listChildInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listChildLegacy: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDiscussionComments: {
            deprecated: string;
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDiscussionCommentsInOrg: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDiscussionCommentsLegacy: {
            deprecated: string;
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDiscussions: {
            deprecated: string;
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDiscussionsInOrg: {
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listDiscussionsLegacy: {
            deprecated: string;
            method: string;
            params: {
                direction: {
                    enum: string[];
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listMembers: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listMembersInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listMembersLegacy: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                role: {
                    enum: string[];
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPendingInvitations: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPendingInvitationsInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPendingInvitationsLegacy: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listProjects: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listProjectsInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listProjectsLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listRepos: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReposInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listReposLegacy: {
            deprecated: string;
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMember: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMemberLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMembership: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMembershipInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeMembershipLegacy: {
            deprecated: string;
            method: string;
            params: {
                team_id: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProject: {
            deprecated: string;
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProjectInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeProjectLegacy: {
            deprecated: string;
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeRepo: {
            deprecated: string;
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeRepoInOrg: {
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        removeRepoLegacy: {
            deprecated: string;
            method: string;
            params: {
                owner: {
                    required: boolean;
                    type: string;
                };
                repo: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        reviewProject: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        reviewProjectInOrg: {
            headers: {
                accept: string;
            };
            method: string;
            params: {
                org: {
                    required: boolean;
                    type: string;
                };
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        reviewProjectLegacy: {
            deprecated: string;
            headers: {
                accept: string;
            };
            method: string;
            params: {
                project_id: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        update: {
            deprecated: string;
            method: string;
            params: {
                description: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                parent_team_id: {
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                privacy: {
                    enum: string[];
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateDiscussion: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        updateDiscussionComment: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateDiscussionCommentInOrg: {
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateDiscussionCommentLegacy: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    required: boolean;
                    type: string;
                };
                comment_number: {
                    required: boolean;
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateDiscussionInOrg: {
            method: string;
            params: {
                body: {
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        updateDiscussionLegacy: {
            deprecated: string;
            method: string;
            params: {
                body: {
                    type: string;
                };
                discussion_number: {
                    required: boolean;
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        updateInOrg: {
            method: string;
            params: {
                description: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                org: {
                    required: boolean;
                    type: string;
                };
                parent_team_id: {
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                privacy: {
                    enum: string[];
                    type: string;
                };
                team_slug: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateLegacy: {
            deprecated: string;
            method: string;
            params: {
                description: {
                    type: string;
                };
                name: {
                    required: boolean;
                    type: string;
                };
                parent_team_id: {
                    type: string;
                };
                permission: {
                    enum: string[];
                    type: string;
                };
                privacy: {
                    enum: string[];
                    type: string;
                };
                team_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
    };
    users: {
        addEmails: {
            method: string;
            params: {
                emails: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        block: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkBlocked: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkFollowing: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        checkFollowingForUser: {
            method: string;
            params: {
                target_user: {
                    required: boolean;
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        createGpgKey: {
            method: string;
            params: {
                armored_public_key: {
                    type: string;
                };
            };
            url: string;
        };
        createPublicKey: {
            method: string;
            params: {
                key: {
                    type: string;
                };
                title: {
                    type: string;
                };
            };
            url: string;
        };
        deleteEmails: {
            method: string;
            params: {
                emails: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deleteGpgKey: {
            method: string;
            params: {
                gpg_key_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        deletePublicKey: {
            method: string;
            params: {
                key_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        follow: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getAuthenticated: {
            method: string;
            params: {};
            url: string;
        };
        getByUsername: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getContextForUser: {
            method: string;
            params: {
                subject_id: {
                    type: string;
                };
                subject_type: {
                    enum: string[];
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getGpgKey: {
            method: string;
            params: {
                gpg_key_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        getPublicKey: {
            method: string;
            params: {
                key_id: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        list: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                since: {
                    type: string;
                };
            };
            url: string;
        };
        listBlocked: {
            method: string;
            params: {};
            url: string;
        };
        listEmails: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listFollowersForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listFollowersForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listFollowingForAuthenticatedUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listFollowingForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listGpgKeys: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listGpgKeysForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        listPublicEmails: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicKeys: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
            };
            url: string;
        };
        listPublicKeysForUser: {
            method: string;
            params: {
                page: {
                    type: string;
                };
                per_page: {
                    type: string;
                };
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        togglePrimaryEmailVisibility: {
            method: string;
            params: {
                email: {
                    required: boolean;
                    type: string;
                };
                visibility: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unblock: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        unfollow: {
            method: string;
            params: {
                username: {
                    required: boolean;
                    type: string;
                };
            };
            url: string;
        };
        updateAuthenticated: {
            method: string;
            params: {
                bio: {
                    type: string;
                };
                blog: {
                    type: string;
                };
                company: {
                    type: string;
                };
                email: {
                    type: string;
                };
                hireable: {
                    type: string;
                };
                location: {
                    type: string;
                };
                name: {
                    type: string;
                };
            };
            url: string;
        };
    };
};
export default _default;
