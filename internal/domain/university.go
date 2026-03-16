package domain

type University struct {
	ID   string
	Name string
}

var SupportedUniversities = []University{
	{ID: "igktu", Name: "ИГХТУ"},
	{ID: "igu", Name: "ИГУ"},
	{ID: "igeu", Name: "ИГЭУ"},
}
