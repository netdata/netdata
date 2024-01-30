- Install Dagger CLI:
  ```
  cd /usr/local
  curl -L https://dl.dagger.io/dagger/install.sh | sudo sh
  ```
- Install python requirements:
  ```
  pip install -r packaging/dag/requirements.txt
  ```

Now you can run something like this:

```
dagger run python packaging/dag/main.py build -p linux/x86_64 -d debian12
```

or

```
dagger run python packaging/dag/main.py test
```.


