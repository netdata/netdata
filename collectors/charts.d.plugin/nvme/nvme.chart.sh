# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
#
# Made by Alexandre Azedo | SRE <aazedo@gocontact.pt>
# 2021

# _update_every is a special variable - it holds the number of seconds
# between the calls of the _update() function
nvme_update_every=5

# the priority is used to sort the charts on the dashboard
# 1 = the first chart
nvme_priority=49000

# global variables to store our collected data
critical_warning=
percentage_used=
temperature=
power_cycles=
power_on_hours=

nvme_get() {
  # do all the work to collect / calculate the values
  # for each dimension
  #
  # Remember:
  # 1. KEEP IT SIMPLE AND SHORT
  # 2. AVOID FORKS (avoid piping commands)
  # 3. AVOID CALLING TOO MANY EXTERNAL PROGRAMS
  # 4. USE LOCAL VARIABLES (global variables may overlap with other modules)
  nvme_list=$(sudo -n nvme list | awk '/nvme/ { print $1 }')
  local i=
  for i in ${nvme_list[*]};do
    
    h=${i/\/dev\//}
	
    # read the critical warning in the nvme
    critical_warning+="SET ${h} =$(sudo -n nvme smart-log ${i} | awk -F: '/critical_warning/ { print $2 }')"$'\n'
	
    # read the percentage used in the nvme	
    percentage_used+="SET ${h} =$(sudo -n nvme smart-log ${i} | awk -F: '/percentage_used/ { print $2}' | awk -F% '{print $1}')"$'\n'
	
    # read the temperature in the nvme
    temperature+="SET ${h} = $(sudo -n nvme smart-log ${i} | awk -F: '/temperature/ { print $2 }' | awk '{ print $1 }')"$'\n'
	
    # read the power cycles in the nvme	
    power_cycles+="SET ${h} =$(sudo -n nvme smart-log ${i} | awk -F: '/power_cycles/ { print $2 }')"$'\n'
	
    # read the power on hours in the nvme
    power_on_hours+="SET ${h} =$(sudo -n nvme smart-log ${i} | awk -F: '/power_on_hours/ { print $2 }' | awk -F, '{ print $1 }')"$'\n'
    
  done
  # this should return:
  #  - 0 to send the data to netdata
  #  - 1 to report a failure to collect the data

  return 0
}

# This function checks if the module can collect data (or checks its config).
# It's run only once for the lifetime of the module.
# It returns 0 or 1 depending on whether the module is able to run or not (0=OK, 1=FAILED).
nvme_check() {
  # this should return:
  #  - 0 to enable the chart
  #  - 1 to disable the chart

  # check something
  require_cmd awk || return 1
  require_cmd grep || return 1
  require_cmd nvme || return 1

  # check that we can collect data
  nvme_get || return 1

  return 0
}

# This function creates the charts and dimensions using the standard Netdata plugin.
# It's called just once and only after unimrcp_check was successful. 
# It returns 0 (OK) or 1 (FAILED), with a non-zero return value disabling the collector.
nvme_create() {

  nvme_dimension=$(sudo -n nvme list | awk '/nvme/ { print $1 }' | awk -F/ '{print $3}' | sed -re "s/(^nvme.*)/DIMENSION \1 '' absolute 1 1/")
  # create the charts
  cat << EOF
CHART nvme.critical_warning '' "Critical Warning" "critical_warning" critical_warning nvme.critical_warning area $((nvme_priority)) $nvme_update_every
$nvme_dimension

CHART nvme.percentage_used '' "Percentage Used" "percentage_used" percentage_used nvme.percentage_used area $((nvme_priority + 1)) $nvme_update_every
$nvme_dimension

CHART nvme.temperature '' "Temperature" "temperature" temperature nvme.temperature area $((nvme_priority + 2)) $nvme_update_every
$nvme_dimension

CHART nvme.power_cycles '' "Power cycles" "power_cycles" power_cycles nvme.power_cycles area $((nvme_priority + 3)) $nvme_update_every
$nvme_dimension

CHART nvme.power_on_hours '' "Power on hours" "power_on_hours" power_on_hours nvme.power_on_hours area $((nvme_priority + 4)) $nvme_update_every
$nvme_dimension

EOF
  return 0
}

# This function collects new values and sends them to Netdata, following the Netdata plugin.
# It's called repeatedly every unimrcp_update_every seconds.
# It's called with one parameter: microseconds since the last time it was run.
# It returns 0 (OK) or 1 (FAILED), with a non-zero return value disabling the collector.
nvme_update() {
  # the first argument to this function is the microseconds since last update
  # pass this parameter to the BEGIN statement (see bellow).

  # write the result of the work.
  cat << VALUESEOF
BEGIN nvme.critical_warning $1
${critical_warning::-1}
END

BEGIN nvme.percentage_used $1
${percentage_used::-1}
END

BEGIN nvme.temperature $1
${temperature::-1}
END

BEGIN nvme.power_cycles $1
${power_cycles::-1}
END

BEGIN nvme.power_on_hours $1
${power_on_hours::-1}
END

VALUESEOF
  
  return 0
}
