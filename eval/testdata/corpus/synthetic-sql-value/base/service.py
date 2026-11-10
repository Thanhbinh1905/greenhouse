import sqlite3
def user(db, name):
    return db.execute("select name from users where name = ?", (name,)).fetchall()
