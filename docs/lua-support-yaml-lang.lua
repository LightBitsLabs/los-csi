local function CodeBlock (elem)
    if elem.c[1][2][1] == "yaml" then
        return pandoc.RawBlock("latex", "\n\\begin{lstlisting}[language=yaml]\n"..elem.text.."\n\\end{lstlisting}\n")
    else
        return elem
    end
end

return { { CodeBlock = CodeBlock } }
