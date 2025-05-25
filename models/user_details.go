package models

type CompanyMember struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	CompanyID string `json:"company_id"`
	Users     struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
	} `json:"users"`
}

type Room struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CompanyID  string    `json:"company_id"`
	CreatedBy  string    `json:"created_by"`
	AccessList *[]string  `json:"access_list"`
	AdminList  *[]string `json:"admin_list"`
}

type UserInfo struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type AuthResponse struct {
	User           UserInfo         `json:"user"`
	CompanyMembers []CompanyMember  `json:"company_members"`
	Rooms          []Room           `json:"rooms"`
}