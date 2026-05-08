"""Allow `python -m sqlitedeploy ...` as an alias for the CLI."""

from sqlitedeploy._resolve import main

if __name__ == "__main__":
    main()
