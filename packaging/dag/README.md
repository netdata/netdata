- Install Dagger CLI:
  ```
  cd /usr/local
  curl -L https://dl.dagger.io/dagger/install.sh | sudo sh
  ```
- Install Python's Dagger SDK:
  ```
  pip install dagger-io
  ```

Now you can run something like this:

```
dagger run python packaging/dag/main.py -c -p linux/x86_64 -p linux/i386 -i debian10 -i debian11 -i debian12
```

This will build *concurrently* the agent for debian 10/11/12 on x86_64 and i386.
The first run will be slow. However, subsequent runs will reuse cached artifacts
and should be much faster.

For more information, check the help message:

```
dagger run python packaging/dag/main.py --help
```
