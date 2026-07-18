// Задача LC06-6: анонимная функция как callback.
//
// Это очень маленькая задача на базовое узнавание:
// функцию можно передать как аргумент, а потом вызвать внутри другой функции.
//
// Нужно реализовать:
//
//	func VisitActiveUsers(users []User, visit func(User))
//
// Правила:
//   - пройти по users в исходном порядке;
//   - для каждого пользователя с Active == true вызвать visit(user);
//   - не печатать ничего внутри VisitActiveUsers;
//   - не возвращать результат, вся логика результата должна быть в callback.
//
// Ожидаемый вывод после правильной реализации:
//
//	OK: active users visited
package main

import (
	"fmt"
	"reflect"
)

type User struct {
	ID     int
	Name   string
	Active bool
}

func VisitActiveUsers(users []User, visit func(User)) {
	// TODO: вызови visit только для active users.
	for _, user := range users {
		if user.Active {
			visit(user)
		}
	}
}

func main() {
	users := []User{
		{ID: 1, Name: "Ann", Active: true},
		{ID: 2, Name: "Bob", Active: false},
		{ID: 3, Name: "Kate", Active: true},
	}
	
	var names []string
	
	VisitActiveUsers(users, func(user User) {
		names = append(names, user.Name)
	})
	
	expect("active users visited", names, []string{"Ann", "Kate"})
}

func expect(name string, got, want []string) {
	if reflect.DeepEqual(got, want) {
		fmt.Println("OK:", name)
		return
	}
	
	fmt.Printf("MISMATCH: %s\n got: %#v\nwant: %#v\n", name, got, want)
}
