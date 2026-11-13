from html import escape
def render(name):
    return "<p>" + escape(name) + "</p>"
