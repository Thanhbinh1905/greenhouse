from service import options
a=options()
a["timeout"]=0
assert options()["timeout"] == 5
