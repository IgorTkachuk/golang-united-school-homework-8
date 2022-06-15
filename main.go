package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

type Arguments map[string]string

type User struct {
	Id    string `json:"id"`
	Email string `json:"email"`
	Age   uint8  `json:"age"`
}

type UserList struct {
	datasource
	list []User
}

type ReadWriteCloseReseter interface {
	io.ReadWriteCloser
	Reset()
}

type fileDS struct {
	file os.File
}

func (fds fileDS) Reset() {
	_ = fds.file.Truncate(0)
	_, _ = fds.file.Seek(0, 0)
}

func (fds fileDS) Read(p []byte) (n int, err error) {
	return fds.file.Read(p)
}

func (fds fileDS) Write(p []byte) (n int, err error) {
	return fds.file.Write(p)
}

func (fds fileDS) Close() error {
	return fds.file.Close()
}

func createDS(f dsInitiator, dsParam string) datasource {
	return func() ReadWriteCloseReseter {
		return f(dsParam)
	}
}

type datasource func() ReadWriteCloseReseter
type dsInitiator func(dsParam string) ReadWriteCloseReseter

func fileDataSource(filename string) ReadWriteCloseReseter {
	var fds fileDS
	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		log.Fatal(err)
	}
	fds.file = *f

	return fds
}

func (ul *UserList) New(ds datasource) *UserList {
	ul.datasource = ds
	return ul
}

func (ul *UserList) Load() *UserList {
	f := ul.datasource()
	defer func() {
		f.Close()
	}()

	userFileContent, _ := io.ReadAll(f)
	json.Unmarshal(userFileContent, &ul.list)

	return ul
}

func (ul *UserList) Save() *UserList {
	f := ul.datasource()
	defer func() {
		f.Close()
	}()

	f.Reset()
	sUserList, _ := json.Marshal(&ul.list)
	f.Write(sUserList)

	return ul
}

func (ul *UserList) List(w io.Writer) *UserList {
	sUserList, _ := json.Marshal(ul.list)
	w.Write(sUserList)

	return ul
}

func (ul *UserList) Add(item string) (*UserList, error) {
	var userItem User
	json.Unmarshal([]byte(item), &userItem)
	if _, err := ul.FindById(userItem.Id); err == nil {
		return &UserList{}, fmt.Errorf("Item with id %s already exists", userItem.Id)
	}
	ul.list = append(ul.list, userItem)

	return ul, nil
}

func (ul *UserList) Remove(id string) (*UserList, error) {
	if _, err := ul.FindById(id); err != nil {
		return &UserList{}, fmt.Errorf("Item with id %s not found", id)
	}
	for i, u := range ul.list {
		if u.Id == id {
			var t []User
			t = append(t, ul.list[:i]...)
			ul.list = append(t, ul.list[i+1:]...)
			break
		}
	}

	return ul, nil
}

func (ul *UserList) FindById(id string) (User, error) {
	for _, u := range ul.list {
		if u.Id == id {
			return u, nil
		}
	}

	return User{}, fmt.Errorf("user with id = %s is not found", id)
}

func getArgs(m map[string]flagParams) Arguments {
	var args Arguments
	args = make(map[string]string)

	for k, v := range m {
		t := k
		flag.Func(k, v.usage, func(s string) error {
			args[t] = s
			return nil
		})
	}

	flag.Parse()

	return args
}

func checkArgs(m map[string]flagParams, args Arguments) error {
	for fl, params := range m {
		if _, ok := args[fl]; (!ok || len(args[fl]) == 0) && params.required {
			return errors.New(params.errorMsg)
		}
		if params.allowedValues != nil && len(params.allowedValues) > 0 {
			isFound := false
			for _, v := range params.allowedValues {
				if v == args[fl] {
					isFound = true
					break
				}
			}
			if !isFound {
				return fmt.Errorf(params.errorValMsg, args[fl])
			}
		}
	}
	return nil
}

func checkOpArgs(arg Arguments, m opArgMap) error {
	requiredFlags := m[arg[operationFlag]]
	for _, f := range requiredFlags {
		if _, ok := arg[f]; !ok || len(arg[f]) == 0 {
			return fmt.Errorf("-%s flag has to be specified", f)
		}
	}

	return nil
}

const idFlag = "id"
const itemFlag = "item"
const filenameFlag = "fileName"
const operationFlag = "operation"
const operationList = "list"
const operationAdd = "add"
const operationRemove = "remove"
const operationFindById = "findById"

type flagParams struct {
	defaultValue, usage   string
	required              bool
	allowedValues         []string
	errorMsg, errorValMsg string
}

var flags = map[string]flagParams{
	operationFlag: {
		defaultValue:  "no",
		usage:         "add user item to dataset",
		required:      true,
		allowedValues: []string{operationList, operationAdd, operationRemove, operationFindById},
		errorMsg:      "-operation flag has to be specified",
		errorValMsg:   "Operation %s not allowed!",
	},
	itemFlag: {
		defaultValue:  "no",
		usage:         "item in json format which represent user entity",
		required:      false,
		allowedValues: []string{},
		errorMsg:      "",
		errorValMsg:   "",
	},
	filenameFlag: {
		defaultValue:  "no",
		usage:         "filename with user database",
		required:      true,
		allowedValues: []string{},
		errorMsg:      "-fileName flag has to be specified",
		errorValMsg:   "",
	},
	idFlag: {
		defaultValue:  "no",
		usage:         "user identification",
		required:      false,
		allowedValues: []string{},
		errorMsg:      "",
		errorValMsg:   "",
	},
}

type opArgMap map[string][]string

var opRequiredFlagsMap = opArgMap{
	operationAdd:      []string{itemFlag},
	operationRemove:   []string{idFlag},
	operationFindById: []string{idFlag},
}

func Perform(args Arguments, writer io.Writer) error {
	err := checkArgs(flags, args)
	if err != nil {
		return err
	}

	err = checkOpArgs(args, opRequiredFlagsMap)
	if err != nil {
		return err
	}

	var u UserList
	ds := createDS(fileDataSource, args[filenameFlag])
	u.New(ds).Load()

	switch args[operationFlag] {
	case operationList:
		u.List(writer)
	case operationAdd:
		_, addErr := u.Add(args[itemFlag])
		if addErr != nil {
			writer.Write([]byte(addErr.Error()))
		}
	case operationRemove:
		_, removeErr := u.Remove(args[idFlag])
		if removeErr != nil {
			writer.Write([]byte(removeErr.Error()))
		}
	case operationFindById:
		u, findErr := u.FindById(args[idFlag])
		if findErr != nil {
			writer.Write([]byte(""))
		} else {
			out, _ := json.Marshal(u)
			writer.Write(out)
		}
	default:
		return fmt.Errorf(">> Requested unknown operation")
	}

	if err != nil {
		return err
	}

	u.Save()
	return nil
}

func main() {
	err := Perform(getArgs(flags), os.Stdout)
	if err != nil {
		panic(err)
	}
}
