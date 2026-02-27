#!/usr/bin/expect -f
set password [lindex $argv 0]
set command [lindex $argv 1]

spawn ssh -o StrictHostKeyChecking=no cm@10.10.30.140 $command
expect {
    "password:" {
        send "$password\r"
        expect {
            "Sorry" { exit 1 }
            "password:" { 
                send "$password\r"
                expect "password:"
                exit 1
            }
            "$ "
        }
    }
    "sudo" {
        expect "password:"
        send "$password\r"
        expect "$ "
    }
}
expect eof
