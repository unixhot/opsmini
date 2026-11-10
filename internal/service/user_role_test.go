package service

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// 角色名写错必须报错。
//
// 不校验时用户照样建成功，然后权限为空：登录得进去，菜单只显示不需要权限的
// 页面，任何受控动作一律 403。管理员以为「只读」就写成了 viewer，实际上这个
// 角色根本不存在——查都不用查，因为系统一句提示都没有。
func TestCreateUserRejectsUnknownRole(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Role{}); err != nil {
		t.Fatal(err)
	}
	roleRepo := repository.NewRoleRepo(db)
	if err := roleRepo.Seed(); err != nil {
		t.Fatalf("seed roles: %v", err)
	}
	svc := NewUserService(repository.NewUserRepo(db))
	svc.SetRoleRepo(roleRepo)

	if _, err := svc.Create(CreateUserReq{Username: "u1", Password: "123456", Role: "viewer"}); err == nil {
		t.Fatal("不存在的角色被接受了")
	}
	if _, err := svc.Create(CreateUserReq{Username: "u2", Password: "123456", Role: model.RoleReadonly}); err != nil {
		t.Fatalf("内置角色被拒: %v", err)
	}
	// 不传角色时默认 operator，要继续能用。
	if _, err := svc.Create(CreateUserReq{Username: "u3", Password: "123456"}); err != nil {
		t.Fatalf("默认角色被拒: %v", err)
	}
	// 编辑时同样要校验。
	u, err := svc.Create(CreateUserReq{Username: "u4", Password: "123456", Role: model.RoleReadonly})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(u.ID, UpdateUserReq{Role: "nope"}); err == nil {
		t.Fatal("编辑到不存在的角色被接受了")
	}
	// 重名要给能看懂的话，而不是把 SQLite 的约束错误原样抛给用户。
	if _, err := svc.Create(CreateUserReq{Username: "u2", Password: "123456"}); err == nil {
		t.Fatal("重名被接受了")
	} else {
		var taken *ErrUsernameTaken
		if !errors.As(err, &taken) {
			t.Fatalf("重名错误类型不对: %v", err)
		}
	}
}
