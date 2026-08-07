with open('observe/observe.go', 'r') as f:
    content = f.read()

replacement = """	htmlFuncs["conclusionBadge"] = func(conclusion string) template.HTML {
		text := conclusionText(conclusion)
		cssClass := strings.ReplaceAll(text, " ", "-")
		return template.HTML(fmt.Sprintf(`<span class="rt-badge rt-%s">%s</span>`, cssClass, text))
	}
	htmlFuncs["lower"] = strings.ToLower"""

content = content.replace("""	htmlFuncs["conclusionBadge"] = func(conclusion string) template.HTML {
		text := conclusionText(conclusion)
		cssClass := strings.ReplaceAll(text, " ", "-")
		return template.HTML(fmt.Sprintf(`<span class="rt-badge rt-%s">%s</span>`, cssClass, text))
	}""", replacement)

with open('observe/observe.go', 'w') as f:
    f.write(content)
