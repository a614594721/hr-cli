import os, pymysql

c = pymysql.connect(
    host=os.environ['DB_HOST'],
    user=os.environ['DB_USER'],
    password=os.environ['DB_PASSWORD'],
    database=os.environ['DB_NAME'],
    charset='utf8mb4',
)
cur = c.cursor()

# 看 eEmployee_Work 关键列 + 行数
print("=== eEmployee_Work ===")
cur.execute("SELECT COUNT(*) FROM eEmployee_Work")
print("rows:", cur.fetchone()[0])
cur.execute("SHOW COLUMNS FROM eEmployee_Work")
for col in cur.fetchall():
    print(f"  {col[0]:<25} {col[1]:<22} null={col[2]}")

# eSP_EmpChangeStart 入参
print("\n=== eSP_EmpChangeStart 参数 ===")
cur.execute("""SELECT parameter_name, data_type, parameter_mode
    FROM information_schema.parameters
    WHERE specific_schema=%s AND specific_name='eSP_EmpChangeStart'
    ORDER BY ordinal_position""", (os.environ['DB_NAME'],))
for r in cur.fetchall():
    print(f"  {r[2]:<6} {r[0]:<15} {r[1]}")

# personal_info 列
print("\n=== personal_info 列 ===")
cur.execute("SHOW COLUMNS FROM personal_info")
for col in cur.fetchall():
    print(f"  {col[0]:<25} {col[1]:<22}")

# users 列
print("\n=== users 列 ===")
cur.execute("SHOW COLUMNS FROM users")
for col in cur.fetchall():
    print(f"  {col[0]:<25} {col[1]:<22}")

# employee_dingding
print("\n=== employee_dingding 列 + 行数 ===")
cur.execute("SELECT COUNT(*) FROM employee_dingding")
print("rows:", cur.fetchone()[0])
cur.execute("SHOW COLUMNS FROM employee_dingding")
for col in cur.fetchall():
    print(f"  {col[0]:<25} {col[1]:<22}")

c.close()
