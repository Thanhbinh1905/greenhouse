def visible(row, tenant):
    return row["tenant"] == tenant and not row["deleted"]
