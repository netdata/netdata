import random

update_every=10

def check():
    return True

def create():
    print("CHART python_example.random '' 'A random number' 'random number' random random line 90000 1")
    print("DIMENSION random1 '' absolute 1 1")
    return True

def update(interval):
    print("BEGIN python_example.random "+str(interval))
    print("SET random1 = "+str(random.randint(0,100)))
    print("END")
    return True
