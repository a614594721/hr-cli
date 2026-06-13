import os, pymysql

c = pymysql.connect(
    host=os.environ['DB_HOST'],
    user=os.environ['DB_USER'],
    password=os.environ['DB_PASSWORD'],
    database=os.environ['DB_NAME'],
    charset='utf8mb4',
)
cur = c.cursor()

def show(t):
    print(f"\n========== {t} ==========")
    try:
        cur.execute(f"SELECT COUNT(*) FROM `{t}`")
        n = cur.fetchone()[0]
        print(f"rows: {n}")
    except Exception as e:
        print(f"count err: {e}")
        return
    try:
        cur.execute(f"SHOW COLUMNS FROM `{t}`")
        cols = cur.fetchall()
        print(f"columns: {len(cols)}")
        for col in cols[:60]:
            print(f"  {col[0]:<30} {col[1]:<25} null={col[2]:<4} key={col[3]:<4}")
        if len(cols) > 60:
            print(f"  ... {len(cols)-60} more columns")
    except Exception as e:
        print(f"col err: {e}")

# 主员工表
show("eemployee")
# 部门/组织
show("odepartment")
show("ocompany")
# 调动相关
show("echange")
show("eemp_change")
show("eemployee_emp")
# 字典
show("ecd_empchangetype")
show("ecd_empchangereason")
show("ecd_orgchangetype")

c.close()
