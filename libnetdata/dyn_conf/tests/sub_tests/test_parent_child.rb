class ParentChildTest
    @@plugin_cfg = <<~HEREDOC
{ "test" : "true" }
HEREDOC
    @@plugin_cfg2 = <<~HEREDOC
{ "asdfgh" : "asdfgh" }
HEREDOC

    @@job_cfg = <<~HEREDOC
{ "i am newly created job" : "true" }
HEREDOC

    def initialize
        @parent = $config[:http_endpoints][:parent]
        @child  = $config[:http_endpoints][:child]
        @plugin = $config[:global][:test_plugin_name]
        @arry_mod = $config[:global][:test_array_module_name]
        @single_mod = $config[:global][:test_single_module_name]
        @test_job = $config[:global][:test_job_name]
    end
    def check_test_plugin_modules_list(host, child = nil)
        rc = DynCfgHttpClient.get_plugin_module_list(host, @plugin, child)
        assert_eq(rc.code, 200, "as HTTP code for get_module_list request on plugin \"#{@plugin}\"")
        modules = nil
        assert_nothing_raised do
            modules = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(modules, :modules)
        assert_eq(modules[:modules].count, 2, "as number of modules in plugin \"#{@plugin}\"")
        modules[:modules].each do |m|
            assert_has_key?(m, :name)
            assert_has_key?(m, :type)
            assert_is_one_of(m[:type], "job_array", "single")
        end
        assert_eq_str(modules[:modules][0][:name], @arry_mod, "name of first module in plugin \"#{@plugin}\"")
        assert_eq_str(modules[:modules][1][:name], @single_mod, "name of second module in plugin \"#{@plugin}\"")
    end
    def run
        TEST_SUITE("Parent/Child plugin config")

        TEST("parent/child/get_plugin_list", "Get child (hops:1) plugin list trough parent")
        plugins = DynCfgHttpClient.get_plugin_list(@parent, @child)
        assert_eq(plugins.code, 200, "as HTTP code for get_plugin_list request")
        assert_nothing_raised do
            plugins = JSON.parse(plugins.parsed_response, symbolize_names: true)
        end
        assert_has_key?(plugins, :configurable_plugins)
        assert_array_include?(plugins[:configurable_plugins], @plugin)
        PASS()

        TEST("parent/child/(set/get)plugin_config", "Set then get and compare child (hops:1) plugin config trough parent")
        rc = DynCfgHttpClient.set_plugin_config(@parent, @plugin, @@plugin_cfg, @child)
        assert_eq(rc.code, 200, "as HTTP code for set_plugin_config request")

        rc = DynCfgHttpClient.get_plugin_config(@parent, @plugin, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_plugin_config request")
        assert_eq_str(rc.parsed_response.chomp!, @@plugin_cfg, "as plugin config")

        # We do this twice with different configs to ensure first config was not loaded from persistent storage (from previous tests)
        rc = DynCfgHttpClient.set_plugin_config(@parent, @plugin, @@plugin_cfg2, @child)
        assert_eq(rc.code, 200, "as HTTP code for set_plugin_config request 2")

        rc = DynCfgHttpClient.get_plugin_config(@parent, @plugin, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_plugin_config request 2")
        assert_eq_str(rc.parsed_response.chomp!, @@plugin_cfg2, "set/get plugin config 2")
        PASS()

        TEST("child/get_plugin_config", "Get child (hops:0) plugin config and compare with what we got trough parent (set_plugin_config from previous test)")
        rc = DynCfgHttpClient.get_plugin_config(@child, @plugin, nil)
        assert_eq(rc.code, 200, "as HTTP code for get_plugin_config request")
        assert_eq_str(rc.parsed_response.chomp!, @@plugin_cfg2.chomp, "as plugin config")
        PASS()

        TEST("parent/child/plugin_module_list", "Get child (hops:1) plugin module list trough parent and check its contents")
        check_test_plugin_modules_list(@parent, @child)
        PASS()

        TEST("child/plugin_module_list", "Get child (hops:0) plugin module list directly and check its contents")
        check_test_plugin_modules_list(@child, nil)
        PASS()

        TEST("parent/child/module/jobs", "Get list of jobs from child (hops:1) trough parent and check its contents, check job updates")
        rc = DynCfgHttpClient.get_job_list(@parent, @plugin, @arry_mod, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        new_job = jobs[:jobs].find {|i| i[:name] == @test_job}
        assert_not_nil(new_job)
        assert_has_key?(new_job, :status)
        assert_not_eq_str(new_job[:status], "unknown", "job status is other than unknown")
        assert_has_key?(new_job, :flags)
        assert_array_include?(new_job[:flags], "JOB_FLG_STREAMING_PUSHED")
        PASS()

        TEST("child/module/jobs", "Get list of jobs direct from child (hops:0) and check its contents, check job updates")
        rc = DynCfgHttpClient.get_job_list(@child, @plugin, @arry_mod, nil)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        new_job = jobs[:jobs].find {|i| i[:name] == @test_job}
        assert_not_nil(new_job)
        assert_has_key?(new_job, :status)
        assert_not_eq_str(new_job[:status], "unknown", "job status is other than unknown")
        assert_has_key?(new_job, :flags)

        assert_array_not_include?(new_job[:flags], "JOB_FLG_STREAMING_PUSHED") # this is plugin directly at child so it should not show this flag
        PASS()

        TEST("parent/child/single_module/jobs", "Attempt getting list of jobs from child (hops:1) trough parent on single module. Check it fails properly")
        rc = DynCfgHttpClient.get_job_list(@parent, @plugin, @single_mod, @child)
        assert_eq(rc.code, 400, "as HTTP code for get_jobs request")
        assert_eq_str(rc.parsed_response, '400 - this module is not array type', "as HTTP code for get_jobs request on single module")
        PASS()

        created_job = SecureRandom.uuid
        TEST("parent/child/module/cr_del_job", "Create and delete job on child (hops:1) trough parent")
        # create new job
        rc = DynCfgHttpClient.create_job(@parent, @plugin, @arry_mod, created_job, @@job_cfg, @child)
        assert_eq_http_code(rc, 200, "as HTTP code for create_job request")
        # check this job is in job list @parent
        rc = DynCfgHttpClient.get_job_list(@parent, @plugin, @arry_mod, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        new_job = jobs[:jobs].find {|i| i[:name] == created_job}
        assert_not_nil(new_job)
        # check this job is in job list @child
        rc = DynCfgHttpClient.get_job_list(@child, @plugin, @arry_mod, nil)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        new_job = jobs[:jobs].find {|i| i[:name] == created_job}
        assert_not_nil(new_job)
        # check we can get job config back
        rc = DynCfgHttpClient.get_job_config(@parent, @plugin, @arry_mod, created_job, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_job_config request")
        assert_eq_str(rc.parsed_response.chomp!, @@job_cfg, "as job config")
        # delete job
        rc = DynCfgHttpClient.delete_job(@parent, @plugin, @arry_mod, created_job, @child)
        assert_eq(rc.code, 200, "as HTTP code for delete_job request")
        # Check it is not in parents job list anymore
        rc = DynCfgHttpClient.get_job_list(@parent, @plugin, @arry_mod, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        new_job = jobs[:jobs].find {|i| i[:name] == created_job}
        assert_nil(new_job)
        # Check it is not in childs job list anymore
        rc = DynCfgHttpClient.get_job_list(@child, @plugin, @arry_mod, nil)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        new_job = jobs[:jobs].find {|i| i[:name] == created_job}
        assert_nil(new_job)
        PASS()

        TEST("parent/child/module/del_undeletable_job", "Try delete job on child (child rejects), check failure case works (hops:1)")
        # test if plugin rejects job deletion the job still remains in list as it should
        rc = DynCfgHttpClient.delete_job(@parent, @plugin, @arry_mod, @test_job, @child)
        assert_eq(rc.code, 500, "as HTTP code for delete_job request")
        rc = DynCfgHttpClient.get_job_list(@parent, @plugin, @arry_mod, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_jobs request")
        jobs = nil
        assert_nothing_raised do
            jobs = JSON.parse(rc.parsed_response, symbolize_names: true)
        end
        assert_has_key?(jobs, :jobs)
        job = jobs[:jobs].find {|i| i[:name] == @test_job}
        assert_not_nil(job)
        PASS()
    end
end

ParentChildTest.new.run()
