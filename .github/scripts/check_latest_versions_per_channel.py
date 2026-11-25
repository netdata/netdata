import check_latest_versions
import modules.version_manipulation as ndvm
import sys

if __name__ == "__main__":
    channel = sys.argv[1]
    sorted_agents_by_major = ndvm.sort_and_grouby_major_agents_of_channel(channel)
    latest_per_major = [values[0] for values in sorted_agents_by_major.values()]
    check_latest_versions.main(latest_per_major)
