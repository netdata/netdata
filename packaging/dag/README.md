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
dagger run python packaging/dag/main.py build -p linux/x86_64 -d debian12
```

or

```
dagger run python packaging/dag/main.py test
```.


