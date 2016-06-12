import random

update_every = 5
priority = 150000

def check():
    return True

def create():
    print("CHART example.python_random '' 'A random number' 'random number' random random line "+str(priority)+" 1")
    print("DIMENSION random1 '' absolute 1 1")
    return True

def update(interval):
    print("BEGIN example.python_random "+str(interval))
    print("SET random1 = "+str(random.randint(0,100)))
    print("END")
    return True
