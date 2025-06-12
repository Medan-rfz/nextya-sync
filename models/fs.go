package models

import "time"

type File struct {
	Path     string
	Modified time.Time
}

type Folder struct {
	Path     string
	Modified time.Time
	Files    []File
	Folders  []Folder
}
