### Understand the alert

The `riakkv_vm_high_process_count` alert is related to the Riak KV database. It warns you when the number of processes running in the Erlang VM is high. High process counts can result in performance degradation due to scheduling overhead.

This alert is triggered in the warning state when the number of processes is greater than 10,000 and in the critical state when it is greater than 100,000.

### Troubleshoot the alert

1. Check the current number of processes in the Erlang VM. You can use the following command to see the active processes:

   ```
   riak-admin status | grep vnode_management_procs
   ```

2. Check the Riak KV logs (/var/log/riak) to see if there are any error messages or stack traces. This can help you identify issues and potential bottlenecks in your system.

3. Check the CPU, memory, and disk space usage on the system hosting the Riak KV database. High usage in any of these areas can also contribute to performance issues and the high process count. Use commands like `top`, `free`, and `df` to monitor these resources.

4. Review your Riak KV configuration settings. You may need to adjust the `+P` and `+S` flags, which control the maximum number of processes and scheduler threads (respectively) that the Erlang runtime system can create. These settings can be found in the `vm.args` file.

   ```
   vim /etc/riak/vm.args
   ```

5. If needed, optimize the Riak KV database by adjusting the configuration settings or by adding more resources to your system, such as RAM or CPU cores.

6. Ensure that your application is not creating an excessive number of processes. You may need to examine your code and see if there are any ways to reduce the Riak KV process count.

### Useful resources

1. [Riak KV Documentation](http://docs.basho.com/riak/kv/2.2.3/)
