from setuptools import setup, find_namespace_packages

setup(
    name="cli-anything-gitmanager",
    version="0.1.0",
    description="CLI harness for Git Manager desktop app",
    packages=find_namespace_packages(include=["cli_anything.*"]),
    package_data={
        "cli_anything.gitmanager": ["skills/*.md"],
    },
    install_requires=[
        "click>=8.0",
    ],
    entry_points={
        "console_scripts": [
            "cli-anything-gitmanager=cli_anything.gitmanager.gitmanager_cli:cli",
        ],
    },
    python_requires=">=3.10",
)
