#!/bin/sh

example_update_every=

example_check() {
	# this should return:
	#  - 0 to enable the chart
	#  - 1 to disable the chart

	return 0
}

example_create() {
# create the chart with 3 dimensions
cat <<EOF
CHART example.random '' "Random Numbers Stacked Chart" "% of random numbers" random random stacked 5000 $example_update_every
DIMENSION random1 '' percentage-of-absolute-row 1 1
DIMENSION random2 '' percentage-of-absolute-row 1 1
DIMENSION random3 '' percentage-of-absolute-row 1 1
EOF

	return 0
}

example_update() {
	# the first argument to this function is the microseconds since last update
	# pass this parameter to the BEGIN statement (see bellow).

	# do all the work to collect / calculate the values
	# for each dimension
	# remember: KEEP IT SIMPLE AND SHORT

	value1=$RANDOM
	value2=$RANDOM
	value3=$RANDOM

	# write the result of the work.
	cat <<VALUESEOF
BEGIN example.random $1
SET random1 = $value1
SET random2 = $value2
SET random3 = $value3
END
VALUESEOF

	return 0
}

