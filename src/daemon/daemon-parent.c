// SPDX-License-Identifier: GPL-3.0-or-later

#include "daemon-parent.h"
#include "status-file.h"

static volatile pid_t netdata_child_pid = 0;
static volatile bool parent_exiting = false;

// Custom signal handler for the parent process
static void parent_signal_handler(int signo) {
    // Forward termination signals to the child process
    // Don't log anything here as we've closed all file descriptors
    
    // Only forward signals if we have a valid child PID and we're not already exiting
    if (netdata_child_pid > 0 && !parent_exiting) {
        // These are termination signals that should be forwarded
        if (signo == SIGINT || signo == SIGTERM || signo == SIGQUIT) {
            // Forward the signal to the child
            kill(netdata_child_pid, signo);
            
            // Mark that we're in the process of exiting
            parent_exiting = true;
        }
    }
}

// Set up signal handling for the parent process to ignore all signals
static void parent_setup_signal_handlers(void) {
    struct sigaction sa;
    sa.sa_handler = parent_signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;

    // Install custom signal handler for all signals that netdata handles
    // Handle standard termination signals
    sigaction(SIGINT, &sa, NULL);
    sigaction(SIGQUIT, &sa, NULL);
    sigaction(SIGTERM, &sa, NULL);
    
    // Handle log and health reload signals
    sigaction(SIGHUP, &sa, NULL);
    sigaction(SIGUSR1, &sa, NULL);
    sigaction(SIGUSR2, &sa, NULL);
    
    // Handle deadly signals
    sigaction(SIGPIPE, &sa, NULL);
    sigaction(SIGBUS, &sa, NULL);
    sigaction(SIGSEGV, &sa, NULL);
    sigaction(SIGFPE, &sa, NULL);
    sigaction(SIGILL, &sa, NULL);
    sigaction(SIGABRT, &sa, NULL);
    sigaction(SIGSYS, &sa, NULL);
    sigaction(SIGXCPU, &sa, NULL);
    sigaction(SIGXFSZ, &sa, NULL);
}

// Change process name to avoid being killed by pkill/killall
static void parent_change_process_name(int argc, char **argv) {
    // On Linux, we can use prctl to change the process name
#ifdef PR_SET_NAME
    prctl(PR_SET_NAME, "nd_watcher", 0, 0, 0);
#endif

    // Update argv[0] which is what ps and other tools show
    if (argv && argc > 0) {
        // First, get length of argv[0] to know how much we can write
        size_t size0 = strlen(argv[0]) + 1;
        memset(argv[0], 0, size0);
        strcatz(argv[0], 0, "nd_watcher", size0);
        
        // Clear all other arguments to avoid showing original parameters
        for (int i = 1; i < argc; i++) {
            if (argv[i]) {
                size_t size = strlen(argv[i]) + 1;
                memset(argv[i], 0, size);
            }
        }
    }
}

// Check the exit reason from the status file and update it if needed
static void parent_check_and_update_exit_status(int status) {
    // Variables to prepare for the status file update
    SIGNAL_CODE sig_code = 0;
    EXIT_REASON exit_reason = 0;
    char fatal_function[128] = {0};
    
    if (WIFEXITED(status)) {
        // Process exited with a return code
        exit_reason = EXIT_REASON_HARD_KILLED;
        snprintf(fatal_function, sizeof(fatal_function), 
                "parent_catch(code %d)", WEXITSTATUS(status));
    }
    else if (WIFSIGNALED(status)) {
        // Process was terminated by a signal
        int sig = WTERMSIG(status);
        
        // Get a formatted signal name
        char signal_name[64];
        // Create signal code from signal number and code 0 (we don't have si_code)
        sig_code = signal_code(sig, 0);
        SIGNAL_CODE_2str_h(sig_code, signal_name, sizeof(signal_name));
        
        // Map the signal to the appropriate exit reason
        switch (sig) {
            case SIGINT:
                exit_reason = EXIT_REASON_SIGINT;
                break;
            case SIGQUIT:
                exit_reason = EXIT_REASON_SIGQUIT;
                break;
            case SIGTERM:
                exit_reason = EXIT_REASON_SIGTERM;
                break;
            case SIGBUS:
                exit_reason = EXIT_REASON_SIGBUS;
                break;
            case SIGSEGV:
                exit_reason = EXIT_REASON_SIGSEGV;
                break;
            case SIGFPE:
                exit_reason = EXIT_REASON_SIGFPE;
                break;
            case SIGILL:
                exit_reason = EXIT_REASON_SIGILL;
                break;
            case SIGABRT:
                exit_reason = EXIT_REASON_SIGABRT;
                break;
            case SIGSYS:
                exit_reason = EXIT_REASON_SIGSYS;
                break;
            case SIGXCPU:
                exit_reason = EXIT_REASON_SIGXCPU;
                break;
            case SIGXFSZ:
                exit_reason = EXIT_REASON_SIGXFSZ;
                break;
            default:
                exit_reason = EXIT_REASON_HARD_KILLED;
                break;
        }
        
        snprintf(fatal_function, sizeof(fatal_function), 
                "parent_catch(signal %s)", signal_name);
    }
    
    // Update the status file with the collected information
    daemon_status_file_parent_update(sig_code, exit_reason, fatal_function);
}

// The main function that implements the parent process
int daemon_parent_start(int argc, char **argv) {
    pid_t pid;
    int status;

    // Create a child process
    pid = fork();
    
    if (pid < 0) {
        // Fork failed - allow netdata to continue without parent
        netdata_log_error("Failed to fork netdata parent watcher process, continuing without it");
        return 0;
    }
    
    if (pid == 0) {
        // This is the child (netdata) process
        // Return to continue with normal netdata execution
        return 0;
    }
    
    // This is the parent process
    netdata_child_pid = pid;
    
    // Close all file descriptors
    // This ensures the parent process doesn't interfere with the user experience
    os_close_all_non_std_open_fds_except(NULL, 0, 0);
    
    // Also close stdin/stdout/stderr 
    close(STDIN_FILENO);
    close(STDOUT_FILENO);
    close(STDERR_FILENO);
    
    // Change process name to avoid being killed by pkill/killall
    parent_change_process_name(argc, argv);
    
    // Setup signal handlers to ignore all signals
    parent_setup_signal_handlers();
    
    // Wait for the child to exit
    // If waitpid is interrupted by a signal (EINTR), retry the call
    pid_t ret;
    do {
        ret = waitpid(netdata_child_pid, &status, 0);
    } while (ret == -1 && errno == EINTR);
    
    // Check for other errors
    if (ret == -1) {
        // Check if the child process still exists
        if (kill(netdata_child_pid, 0) == -1) {
            if (errno == ESRCH) {
                // Child process doesn't exist anymore, but waitpid failed
                // It's likely the child was reaped by another process
                // Create a generic status for this case
                SIGNAL_CODE sig_code = 0;
                EXIT_REASON exit_reason = EXIT_REASON_HARD_KILLED;
                const char *fatal_function = "parent_catch(disappeared)";
                daemon_status_file_parent_update(sig_code, exit_reason, fatal_function);
                exit(0);
            }
        }
        
        // Child still exists despite waitpid error, continue waiting
        // This is a loop that will keep checking if the child is running
        while (1) {
            sleep(1);  // Sleep to avoid tight loop
            
            // Try waitpid again
            ret = waitpid(netdata_child_pid, &status, WNOHANG);
            if (ret == netdata_child_pid) {
                // Got status successfully, break out to process it
                break;
            }
            else if (ret == 0) {
                // Child still running, keep waiting
                continue;
            }
            else {
                // Error or other issue
                // Check if child still exists
                if (kill(netdata_child_pid, 0) == -1 && errno == ESRCH) {
                    // Child is gone now, create a generic status for this case
                    SIGNAL_CODE sig_code = 0;
                    EXIT_REASON exit_reason = EXIT_REASON_HARD_KILLED;
                    const char *fatal_function = "parent_catch(disappeared2)";
                    daemon_status_file_parent_update(sig_code, exit_reason, fatal_function);
                    exit(0);
                }
                // Child still exists, keep waiting
            }
        }
    }
    
    // Child has exited
    if (WIFEXITED(status)) {
        // Child exited with a return value, use the same for the parent
        int exit_code = WEXITSTATUS(status);
        exit(exit_code);
    } 
    else if (WIFSIGNALED(status)) {
        // Child was terminated by a signal
        // Update the status file if needed
        parent_check_and_update_exit_status(status);
        exit(128 + WTERMSIG(status)); // Use signal + 128 as exit code (standard practice)
    }
    
    // Default exit
    exit(0);
    
    // Never reached
    return -1;
}