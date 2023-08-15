class ParentChildTest
    @@plugin_cfg = <<~HEREDOC
{ "test" : "true" }
HEREDOC
    @@plugin_cfg2 = <<~HEREDOC
{ "asdfgh" : "asdfgh" }
HEREDOC

    def initialize
        @parent = $config[:http_endpoints][:parent]
        @child  = $config[:http_endpoints][:child]
        @plugin = $config[:global][:test_plugin_name]
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
        asser_eq_str(modules[:modules][0][:name], "module_of_the_future", "name of first module in plugin \"#{@plugin}\"")
        asser_eq_str(modules[:modules][1][:name], "module_of_the_future_single_type", "name of second module in plugin \"#{@plugin}\"")
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
        asser_eq_str(rc.parsed_response.chomp!, @@plugin_cfg, "as plugin config")

        # We do this twice with different configs to ensure first config was not loaded from persistent storage (from previous tests)
        rc = DynCfgHttpClient.set_plugin_config(@parent, @plugin, @@plugin_cfg2, @child)
        assert_eq(rc.code, 200, "as HTTP code for set_plugin_config request 2")

        rc = DynCfgHttpClient.get_plugin_config(@parent, @plugin, @child)
        assert_eq(rc.code, 200, "as HTTP code for get_plugin_config request 2")
        asser_eq_str(rc.parsed_response.chomp!, @@plugin_cfg2, "set/get plugin config 2")
        PASS()

        TEST("child/get_plugin_config", "Get child (hops:0) plugin config and compare with what we got trough parent (set_plugin_config from previous test)")
        rc = DynCfgHttpClient.get_plugin_config(@child, @plugin, nil)
        assert_eq(rc.code, 200, "as HTTP code for get_plugin_config request")
        asser_eq_str(rc.parsed_response.chomp!, @@plugin_cfg2.chomp, "as plugin config")
        PASS()

        TEST("parent/child/plugin_module_list", "Get child (hops:1) plugin module list trough parent and check its contents")
        check_test_plugin_modules_list(@parent, @child)
        PASS()

        TEST("child/plugin_module_list", "Get child (hops:0) plugin module list directly and check its contents")
        check_test_plugin_modules_list(@child, nil)
        PASS()
    end
end

ParentChildTest.new.run()
