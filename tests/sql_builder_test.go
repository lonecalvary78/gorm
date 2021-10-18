package tests_test

import (
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	. "gorm.io/gorm/utils/tests"
	"os"
	"time"
)

func TestRow(t *testing.T) {
	user1 := User{Name: "RowUser1", Age: 1}
	user2 := User{Name: "RowUser2", Age: 10}
	user3 := User{Name: "RowUser3", Age: 20}
	DB.Save(&user1).Save(&user2).Save(&user3)

	row := DB.Table("users").Where("name = ?", user2.Name).Select("age").Row()

	var age int64
	if err := row.Scan(&age); err != nil {
		t.Fatalf("Failed to scan age, got %v", err)
	}

	if age != 10 {
		t.Errorf("Scan with Row, age expects: %v, got %v", user2.Age, age)
	}

	table := "gorm.users"
	if DB.Dialector.Name() != "mysql" {
		table = "users" // other databases doesn't support select with `database.table`
	}

	DB.Table(table).Where(map[string]interface{}{"name": user2.Name}).Update("age", 20)

	row = DB.Table(table+" as u").Where("u.name = ?", user2.Name).Select("age").Row()
	if err := row.Scan(&age); err != nil {
		t.Fatalf("Failed to scan age, got %v", err)
	}

	if age != 20 {
		t.Errorf("Scan with Row, age expects: %v, got %v", user2.Age, age)
	}
}

func TestRows(t *testing.T) {
	user1 := User{Name: "RowsUser1", Age: 1}
	user2 := User{Name: "RowsUser2", Age: 10}
	user3 := User{Name: "RowsUser3", Age: 20}
	DB.Save(&user1).Save(&user2).Save(&user3)

	rows, err := DB.Table("users").Where("name = ? or name = ?", user2.Name, user3.Name).Select("name, age").Rows()
	if err != nil {
		t.Errorf("Not error should happen, got %v", err)
	}

	count := 0
	for rows.Next() {
		var name string
		var age int64
		rows.Scan(&name, &age)
		count++
	}

	if count != 2 {
		t.Errorf("Should found two records")
	}
}

func TestRaw(t *testing.T) {
	user1 := User{Name: "ExecRawSqlUser1", Age: 1}
	user2 := User{Name: "ExecRawSqlUser2", Age: 10}
	user3 := User{Name: "ExecRawSqlUser3", Age: 20}
	DB.Save(&user1).Save(&user2).Save(&user3)

	type result struct {
		Name  string
		Email string
	}

	var results []result
	DB.Raw("SELECT name, age FROM users WHERE name = ? or name = ?", user2.Name, user3.Name).Scan(&results)
	if len(results) != 2 || results[0].Name != user2.Name || results[1].Name != user3.Name {
		t.Errorf("Raw with scan")
	}

	rows, _ := DB.Raw("select name, age from users where name = ?", user3.Name).Rows()
	count := 0
	for rows.Next() {
		count++
	}
	if count != 1 {
		t.Errorf("Raw with Rows should find one record with name 3")
	}

	DB.Exec("update users set name=? where name in (?)", "jinzhu-raw", []string{user1.Name, user2.Name, user3.Name})
	if DB.Where("name in (?)", []string{user1.Name, user2.Name, user3.Name}).First(&User{}).Error != gorm.ErrRecordNotFound {
		t.Error("Raw sql to update records")
	}

	DB.Exec("update users set age=? where name = ?", gorm.Expr("age * ? + ?", 2, 10), "jinzhu-raw")

	var age int
	DB.Raw("select sum(age) from users where name = ?", "jinzhu-raw").Scan(&age)

	if age != ((1+10+20)*2 + 30) {
		t.Errorf("Invalid age, got %v", age)
	}
}

func TestRowsWithGroup(t *testing.T) {
	users := []User{
		{Name: "having_user_1", Age: 1},
		{Name: "having_user_2", Age: 10},
		{Name: "having_user_1", Age: 20},
		{Name: "having_user_1", Age: 30},
	}

	DB.Create(&users)

	rows, err := DB.Select("name, count(*) as total").Table("users").Group("name").Having("name IN ?", []string{users[0].Name, users[1].Name}).Rows()
	if err != nil {
		t.Fatalf("got error %v", err)
	}

	defer rows.Close()
	for rows.Next() {
		var name string
		var total int64
		rows.Scan(&name, &total)

		if name == users[0].Name && total != 3 {
			t.Errorf("Should have one user having name %v", users[0].Name)
		} else if name == users[1].Name && total != 1 {
			t.Errorf("Should have two users having name %v", users[1].Name)
		}
	}
}

func TestQueryRaw(t *testing.T) {
	users := []*User{
		GetUser("row_query_user", Config{}),
		GetUser("row_query_user", Config{}),
		GetUser("row_query_user", Config{}),
	}
	DB.Create(&users)

	var user User
	DB.Raw("select * from users WHERE id = ?", users[1].ID).First(&user)
	CheckUser(t, user, *users[1])
}

func TestDryRun(t *testing.T) {
	user := *GetUser("dry-run", Config{})

	dryRunDB := DB.Session(&gorm.Session{DryRun: true})

	stmt := dryRunDB.Create(&user).Statement
	if stmt.SQL.String() == "" || len(stmt.Vars) != 9 {
		t.Errorf("Failed to generate sql, got %v", stmt.SQL.String())
	}

	stmt2 := dryRunDB.Find(&user, "id = ?", user.ID).Statement
	if stmt2.SQL.String() == "" || len(stmt2.Vars) != 1 {
		t.Errorf("Failed to generate sql, got %v", stmt2.SQL.String())
	}
}

func TestGroupConditions(t *testing.T) {
	type Pizza struct {
		ID   uint
		Name string
		Size string
	}
	dryRunDB := DB.Session(&gorm.Session{DryRun: true})

	stmt := dryRunDB.Where(
		DB.Where("pizza = ?", "pepperoni").Where(DB.Where("size = ?", "small").Or("size = ?", "medium")),
	).Or(
		DB.Where("pizza = ?", "hawaiian").Where("size = ?", "xlarge"),
	).Find(&Pizza{}).Statement

	execStmt := dryRunDB.Exec("WHERE (pizza = ? AND (size = ? OR size = ?)) OR (pizza = ? AND size = ?)", "pepperoni", "small", "medium", "hawaiian", "xlarge").Statement

	result := DB.Dialector.Explain(stmt.SQL.String(), stmt.Vars...)
	expects := DB.Dialector.Explain(execStmt.SQL.String(), execStmt.Vars...)

	if !strings.HasSuffix(result, expects) {
		t.Errorf("expects: %v, got %v", expects, result)
	}
}

func TestCombineStringConditions(t *testing.T) {
	dryRunDB := DB.Session(&gorm.Session{DryRun: true})
	sql := dryRunDB.Where("a = ? or b = ?", "a", "b").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(a = .+ or b = .+\) AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Or("c = ? and d = ?", "c", "d").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(\(a = .+ or b = .+\) OR \(c = .+ and d = .+\)\) AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Or("c = ?", "c").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(\(a = .+ or b = .+\) OR c = .+\) AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Or("c = ? and d = ?", "c", "d").Or("e = ? and f = ?", "e", "f").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(\(a = .+ or b = .+\) OR \(c = .+ and d = .+\) OR \(e = .+ and f = .+\)\) AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Where("c = ? and d = ?", "c", "d").Not("e = ? and f = ?", "e", "f").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(a = .+ or b = .+\) AND \(c = .+ and d = .+\) AND NOT \(e = .+ and f = .+\) AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Where("c = ?", "c").Not("e = ? and f = ?", "e", "f").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(a = .+ or b = .+\) AND c = .+ AND NOT \(e = .+ and f = .+\) AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Where("c = ? and d = ?", "c", "d").Not("e = ?", "e").Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE \(a = .+ or b = .+\) AND \(c = .+ and d = .+\) AND NOT e = .+ AND .users.\..deleted_at. IS NULL`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Where("a = ? or b = ?", "a", "b").Unscoped().Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE a = .+ or b = .+$`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Or("a = ? or b = ?", "a", "b").Unscoped().Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE a = .+ or b = .+$`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}

	sql = dryRunDB.Not("a = ? or b = ?", "a", "b").Unscoped().Find(&User{}).Statement.SQL.String()
	if !regexp.MustCompile(`WHERE NOT \(a = .+ or b = .+\)$`).MatchString(sql) {
		t.Fatalf("invalid sql generated, got %v", sql)
	}
}

func TestFromWithJoins(t *testing.T) {
	var result User

	newDB := DB.Session(&gorm.Session{NewDB: true, DryRun: true}).Table("users")

	newDB.Clauses(
		clause.From{
			Tables: []clause.Table{{Name: "users"}},
			Joins: []clause.Join{
				{
					Table: clause.Table{Name: "companies", Raw: false},
					ON: clause.Where{
						Exprs: []clause.Expression{
							clause.Eq{
								Column: clause.Column{
									Table: "users",
									Name:  "company_id",
								},
								Value: clause.Column{
									Table: "companies",
									Name:  "id",
								},
							},
						},
					},
				},
			},
		},
	)

	newDB.Joins("inner join rgs on rgs.id = user.id")

	stmt := newDB.First(&result).Statement
	str := stmt.SQL.String()

	if !strings.Contains(str, "rgs.id = user.id") {
		t.Errorf("The second join condition is over written instead of combining")
	}

	if !strings.Contains(str, "`users`.`company_id` = `companies`.`id`") && !strings.Contains(str, "\"users\".\"company_id\" = \"companies\".\"id\"") {
		t.Errorf("The first join condition is over written instead of combining")
	}
}

func TestToSQL(t *testing.T) {
	// only test in PostgreSQL
	var dialect = os.Getenv("GORM_DIALECT")
	if dialect != "postgres" {
		t.Skipf("Skipping test for %s database", dialect)
		return
	}

	// By default DB.DryRun should false
	if DB.DryRun != false {
		t.Fatal("Failed expect DB.DryRun to be false")
	}

	date, _ := time.Parse("2006-01-02", "2021-10-18")

	// find
	tx := DB.BuildSQL().Model(&User{}).Where("id = ?", 100).Limit(10).Order("age desc").Find(&[]User{})
	assertEqualSQL(t, `SELECT * FROM "users" WHERE id = $1 AND "users"."deleted_at" IS NULL ORDER BY age desc LIMIT 10`, tx.Statement.SQL.String())
	sql := tx.ToSQL()
	assertEqualSQL(t, `SELECT * FROM "users" WHERE id = 100 AND "users"."deleted_at" IS NULL ORDER BY age desc LIMIT 10`, sql)
	// will reset SQL after build
	assertEqualSQL(t, ``, tx.Statement.SQL.String())

	// after model chagned
	if DB.Statement.DryRun == true || DB.DryRun == true {
		t.Fatal("Failed expect DB.DryRun and DB.Statement.ToSQL to be false")
	}

	// first
	sql = DB.BuildSQL().Model(&User{}).Where(&User{Name: "foo", Age: 20}).Limit(10).Offset(5).Order("name ASC").First(&User{}).ToSQL()
	assertEqualSQL(t, `SELECT * FROM "users" WHERE "users"."name" = 'foo' AND "users"."age" = 20 AND "users"."deleted_at" IS NULL ORDER BY name ASC,"users"."id" LIMIT 1 OFFSET 5`, sql)

	// last and unscoped
	sql = DB.BuildSQL().Model(&User{}).Unscoped().Where(&User{Name: "bar", Age: 12}).Limit(10).Offset(5).Order("name ASC").Last(&User{}).ToSQL()
	assertEqualSQL(t, `SELECT * FROM "users" WHERE "users"."name" = 'bar' AND "users"."age" = 12 ORDER BY name ASC,"users"."id" DESC LIMIT 1 OFFSET 5`, sql)

	// create
	user := &User{Name: "foo", Age: 20}
	user.CreatedAt = date
	user.UpdatedAt = date
	sql = DB.BuildSQL().Model(&User{}).Create(user).ToSQL()
	assertEqualSQL(t, `INSERT INTO "users" ("created_at","updated_at","deleted_at","name","age","birthday","company_id","manager_id","active") VALUES ('2021-10-18 00:00:00','2021-10-18 00:00:00',NULL,'foo',20,NULL,NULL,NULL,false) RETURNING "id"`, sql)

	// save
	user = &User{Name: "foo", Age: 20}
	user.CreatedAt = date
	user.UpdatedAt = date
	sql = DB.BuildSQL().Model(&User{}).Save(user).ToSQL()
	assertEqualSQL(t, `INSERT INTO "users" ("created_at","updated_at","deleted_at","name","age","birthday","company_id","manager_id","active") VALUES ('2021-10-18 00:00:00','2021-10-18 00:00:00',NULL,'foo',20,NULL,NULL,NULL,false) RETURNING "id"`, sql)

	// updates
	user = &User{Name: "bar", Age: 22}
	user.CreatedAt = date
	user.UpdatedAt = date
	sql = DB.BuildSQL().Model(&User{}).Where("id = ?", 100).Updates(user).ToSQL()
	assertEqualSQL(t, `UPDATE "users" SET "created_at"='2021-10-18 00:00:00',"updated_at"='2021-10-18 19:50:09.438',"name"='bar',"age"=22 WHERE id = 100 AND "users"."deleted_at" IS NULL`, sql)

	// update
	sql = DB.BuildSQL().Model(&User{}).Where("id = ?", 100).Update("name", "Foo bar").ToSQL()
	assertEqualSQL(t, `UPDATE "users" SET "name"='Foo bar',"updated_at"='2021-10-18 19:50:09.438' WHERE id = 100 AND "users"."deleted_at" IS NULL`, sql)

	// UpdateColumn
	sql = DB.BuildSQL().Model(&User{}).Where("id = ?", 100).UpdateColumn("name", "Foo bar").ToSQL()
	assertEqualSQL(t, `UPDATE "users" SET "name"='Foo bar' WHERE id = 100 AND "users"."deleted_at" IS NULL`, sql)

	// UpdateColumns
	sql = DB.BuildSQL().Model(&User{}).Where("id = ?", 100).UpdateColumns(User{Name: "Foo", Age: 100}).ToSQL()
	assertEqualSQL(t, `UPDATE "users" SET "name"='Foo',"age"=100 WHERE id = 100 AND "users"."deleted_at" IS NULL`, sql)

	// after model chagned
	if DB.Statement.DryRun == true || DB.DryRun == true {
		t.Fatal("Failed expect DB.DryRun and DB.Statement.ToSQL to be false")
	}
}

func assertEqualSQL(t *testing.T, expected string, actually string) {
	t.Helper()

	// ignore updated_at value
	var updatedAtRe = regexp.MustCompile(`"updated_at"='.+?'`)
	actually = updatedAtRe.ReplaceAllString(actually, `"updated_at"='?'`)
	expected = updatedAtRe.ReplaceAllString(expected, `"updated_at"='?'`)

	if actually != expected {
		t.Fatalf("Failed generate save SQL\nexpected: %s\nactually: %s", expected, actually)
	}
}
