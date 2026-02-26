from setuptools import setup, find_packages

setup(
    name="netdata-tester",
    version="1.0.0",
    packages=find_packages(),
    install_requires=[
        "paramiko>=3.0.0",
    ],
    entry_points={
        "console_scripts": [
            "netdata-tester=netdata_tester.cli:main",
        ],
    },
    python_requires=">=3.8",
)