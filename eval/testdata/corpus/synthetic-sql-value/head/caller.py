import sqlite3
from service import user
db=sqlite3.connect(":memory:")
db.execute("create table users(name text)")
db.execute("insert into users values ('admin')")
assert user(db, "' OR 1=1 --") == []
