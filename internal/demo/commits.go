package demo

type Commit struct {
	Author string
	Handle string
	Hash   string
	Body   string
}

func Commits() []Commit {
	return []Commit{
		{
			Author: "Ada Example",
			Handle: "@ada-demo",
			Hash:   "a1b2c3d",
			Body:   "Shipped a smaller build. Fewer moving parts, more room to think.",
		},
		{
			Author: "Lin Example",
			Handle: "@lin-demo",
			Hash:   "e4f5a6b",
			Body:   "A quiet place for what you are building. One commit at a time.",
		},
	}
}
