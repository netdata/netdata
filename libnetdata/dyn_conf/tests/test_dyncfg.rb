#!/usr/bin/env ruby

require 'json'
require 'httparty'
require 'pastel'
require 'securerandom'

ARGV.length == 1 or raise "Usage: #{$0} <config file>"
config_file = ARGV[0]

File.exist?(config_file) or raise "File not found: #{config_file}"

$config = JSON.parse(File.read(config_file), symbolize_names: true)

$plugin_name = $config[:global][:test_plugin_name]
$pastel = Pastel.new

class TestRunner
    attr_reader :stats
    def initialize
        @stats = {
            :suites => 0,
            :tests  => 0,
            :assertions => 0
        }
        @test = nil
    end
    def add_assertion()
        @stats[:assertions] += 1
    end
    def FAIL(msg, exception = nil, loc = nil)
        puts $pastel.red.bold(" ✕ FAIL")
        STDERR.print "    "
        if loc
            STDERR.print $pastel.yellow("@#{loc.path}:#{loc.lineno}: ")
        else
            STDERR.print $pastel.yellow("@#{caller_locations(1, 1).first.path}:#{caller_locations(1, 1).first.lineno}: ")
        end
        STDERR.puts msg
        STDERR.puts exception.full_message(:highlight => true) if exception
        STDERR.puts $pastel.yellow("    Backtrace:")
        caller.each do |line|
            STDERR.puts "        #{line}"
        end
        exit 1
    end
    def PASS()
        STDERR.puts $pastel.green.bold(" ✓ PASS")
        @stats[:tests] += 1
        @test = nil
    end
    def TEST_SUITE(name)
        puts $pastel.bold("• TEST SUITE: \"#{name}\"")
        @stats[:suites] += 1
    end
    def assert_no_test_running()
        unless @test.nil?
            STDERR.puts $pastel.red("\nFATAL: Test \"#{@test}\" did not call PASS() or FAIL()!")
            exit 1
        end
    end
    def TEST(name, description = nil)
        assert_no_test_running()
        @test = name
        col = 0
        txt = "  ├─ T: #{name} "
        col += txt.length
        print $pastel.bold(txt)

        tab = 50
        rem = tab - (col % tab)
        rem.times do putc ' ' end
        col += rem

        if (description)
            txt = " - #{description} "
            col += txt.length
            print txt

            tab = 180
            rem = tab - (col % tab)
            rem.times do putc '.' end
        end
    end
    def FINALIZE()
        assert_no_test_running()
    end
end

$test_runner = TestRunner.new
def FAIL(msg, exception = nil, loc = nil)
    $test_runner.FAIL(msg, exception, loc)
end
def PASS()
    $test_runner.PASS()
end
def TEST_SUITE(name)
    $test_runner.TEST_SUITE(name)
end
def TEST(name, description = nil)
    $test_runner.TEST(name, description)
end

def assert_eq(got, expected, msg = nil)
    unless got == expected
        FAIL("Expected #{expected}, got #{got} #{msg ? "(#{msg})" : ""}", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_eq_http_code(got, expected, msg = nil)
    unless got.code == expected
        FAIL("Expected #{expected}, got #{got}. Server \"#{got.parsed_response}\" #{msg ? "(#{msg})" : ""}", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_eq_str(got, expected, msg = nil)
    unless got == expected
        FAIL("Strings do not match #{msg ? "(#{msg})" : ""}", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_not_eq_str(got, expected, msg = nil)
    unless got != expected
        FAIL("Strings shoud not match #{msg ? "(#{msg})" : ""}", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_nothing_raised()
    begin
        yield
    rescue Exception => e
        FAIL("Unexpected exception of type #{e.class} raised. Msg: \"#{e.message}\"", e, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_has_key?(hash, key)
    unless hash.has_key?(key)
        FAIL("Expected key \"#{key}\" in hash", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_array_include?(array, value)
    unless array.include?(value)
        FAIL("Expected array to include \"#{value}\"", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_array_not_include?(array, value)
    if array.include?(value)
        FAIL("Expected array to not include \"#{value}\"", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_is_one_of(value, *values)
    unless values.include?(value)
        FAIL("Expected value to be one of #{values.join(", ")}", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_not_nil(value)
    if value.nil?
        FAIL("Expected value to not be nil", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end
def assert_nil(value)
    unless value.nil?
        FAIL("Expected value to be nil", nil, caller_locations(1, 1).first)
    end
    $test_runner.add_assertion()
end


class DynCfgHttpClient
    def self.protocol(cfg)
        return cfg[:ssl] ? 'https://' : 'http://'
    end
    def self.url_base(host)
        return "#{protocol(host)}#{host[:host]}:#{host[:port]}"
    end
    def self.get_url_cfg_base(host, child = nil)
        url = url_base(host)
        url += "/host/#{child[:mguid]}" if child
        url += "/api/v2/config"
        return url
    end
    def self.get_url_cfg_plugin(host, plugin, child = nil)
        return get_url_cfg_base(host, child) + '/' + plugin
    end
    def self.get_url_cfg_module(host, plugin, mod, child = nil)
        return get_url_cfg_plugin(host, plugin, child) + '/' + mod
    end
    def self.get_url_cfg_job(host, plugin, mod, job_id, child = nil)
        return get_url_cfg_module(host, plugin, mod, child) + "/#{job_id}"
    end
    def self.get_plugin_list(host, child = nil)
        begin
            return HTTParty.get(get_url_cfg_base(host, child), verify: false, format: :plain)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.get_plugin_config(host, plugin, child = nil)
        begin
            return HTTParty.get(get_url_cfg_plugin(host, plugin, child), verify: false)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.set_plugin_config(host, plugin, cfg, child = nil)
        begin
            return HTTParty.put(get_url_cfg_plugin(host, plugin, child), verify: false, body: cfg)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.get_plugin_module_list(host, plugin, child = nil)
        begin
            return HTTParty.get(get_url_cfg_plugin(host, plugin, child) + "/modules", verify: false, format: :plain)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.get_job_list(host, plugin, mod, child = nil)
        begin
            return HTTParty.get(get_url_cfg_module(host, plugin, mod, child) + "/jobs", verify: false, format: :plain)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.create_job(host, plugin, mod, job_id, job_cfg, child = nil)
        begin
            return HTTParty.post(get_url_cfg_job(host, plugin, mod, job_id, child), verify: false, body: job_cfg)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.delete_job(host, plugin, mod, job_id, child = nil)
        begin
            return HTTParty.delete(get_url_cfg_job(host, plugin, mod, job_id, child), verify: false)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.get_job_config(host, plugin, mod, job_id, child = nil)
        begin
            return HTTParty.get(get_url_cfg_job(host, plugin, mod, job_id, child), verify: false, format: :plain)
        rescue => e
            FAIL(e.message, e)
        end
    end
    def self.set_job_config(host, plugin, mod, job_id, job_cfg, child = nil)
        begin
            return HTTParty.put(get_url_cfg_job(host, plugin, mod, job_id, child), verify: false, body: job_cfg)
        rescue => e
            FAIL(e.message, e)
        end
    end
end

require_relative 'sub_tests/test_parent_child.rb'

$test_runner.FINALIZE()
puts $pastel.green.bold("All tests passed!")
puts ("Total #{$test_runner.stats[:assertions]} assertions, #{$test_runner.stats[:tests]} tests in #{$test_runner.stats[:suites]} suites")
exit 0
