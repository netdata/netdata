import os


def update_github_env(key, value):
    try:
        env_file = os.getenv('GITHUB_ENV')
        print(env_file)
        with open(env_file, "a") as file:
            file.write(f"{key}={value}")
        print(f"Updated GITHUB_ENV with {key}={value}")
    except Exception as e:
        print(f"Error updating GITHUB_ENV. Error: {e}")


def update_github_output(key, value):
    try:
        env_file = os.getenv('GITHUB_OUTPUT')
        print(env_file)
        with open(env_file, "a") as file:
            file.write(f"{key}={value}")
        print(f"Updated GITHUB_OUTPUT with {key}={value}")
    except Exception as e:
        print(f"Error updating GITHUB_OUTPUT. Error: {e}")


def run_as_github_action():
    return os.environ.get('GITHUB_ACTIONS') == 'true'
