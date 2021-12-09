---
title:  |
  ![](assets/images/lightbits-cover-page.jpg){width=15cm}


  Lightbits CSI Plugin v1.7.0 Deployment Guide
subtitle: |

  LightOS Version: v2.3.12

  Kubernetes Versions: v1.16 - v1.21
author: Lightbits Labs
date: \today
listings: true
numbersections: true
geometry: "left=1.5cm,right=1.5cm,top=1.5cm,bottom=1.5cm"
include-before: |
  \newpage
header-includes: |

  \usepackage[utf8x]{inputenc}
  \usepackage[english]{babel}

  \BeforeBeginEnvironment{lstlisting}{\par\noindent\begin{minipage}{\linewidth}}
  \AfterEndEnvironment{lstlisting}{\end{minipage}\par\addvspace{\topskip}}

  \usepackage{fancyhdr}
  \pagestyle{fancy}
  \fancyfoot[CO,CE]{Proprietary And Confidential}
  \fancyfoot[LE,RO]{Under NDA Only}
  \fancyfoot[LE,LO]{2021 Lightbits Labs}

  \usepackage{listings}
  \usepackage{xcolor}
  
  \definecolor{mygreen}{rgb}{0,0.8,0}
  \definecolor{mygray}{rgb}{0.5,0.5,0.5}
  \definecolor{mymauve}{rgb}{0.58,0,0.82}
  
  \lstdefinelanguage{yaml}{ % taken from: https://github.com/marekaf/docker-lstlisting/blob/master/latex.tex
    keywords={kind, image},
    keywordstyle=\color{blue}\bfseries,
    identifierstyle=\color{black},
    sensitive=false,
    comment=[l]{\#},
    commentstyle=\color{purple}\ttfamily,
    stringstyle=\color{red}\ttfamily,
    morestring=[b]',
    morestring=[b]"
  }

  \lstset{ 
    backgroundcolor=\color{lightgray},   % choose the background color; you must add \usepackage{color} or \usepackage{xcolor}; should come as last argument
    language=bash,                   % the language of the code
    basicstyle=\ttfamily,
    % basicstyle=\footnotesize,        % the size of the fonts that are used for the code
    breakatwhitespace=false,         % sets if automatic breaks should only happen at whitespace
    breaklines=true,                 % sets automatic line breaking
    captionpos=b,                    % sets the caption-position to bottom
    commentstyle=\color{mygreen},    % comment style
    deletekeywords={...},            % if you want to delete keywords from the given language
    escapeinside={\%*}{*)},          % if you want to add LaTeX within your code
    extendedchars=true,              % lets you use non-ASCII characters; for 8-bits encodings only, does not work with UTF-8
    frame=single,                    % adds a frame around the code
    keepspaces=true,                 % keeps spaces in text, useful for keeping indentation of code (possibly needs columns=flexible)
    keywordstyle=\color{blue},       % keyword style
    morekeywords={*,...},            % if you want to add more keywords to the set
    % firstnumber=1000,                % start line enumeration with line 1000
    % numbers=left,                    % where to put the line-numbers; possible values are (none, left, right)
    % numbersep=5pt,                   % how far the line-numbers are from the code
    % numberstyle=\tiny\color{mygray}, % the style that is used for the line-numbers
    rulecolor=\color{black},         % if not set, the frame-color may be changed on line-breaks within not-black text (e.g. comments (green here))
    showspaces=false,                % show spaces everywhere adding particular underscores; it overrides 'showstringspaces'
    showstringspaces=false,          % underline spaces within strings only
    showtabs=false,                  % show tabs within strings adding particular underscores
    stepnumber=2,                    % the step between two line-numbers. If it's 1, each line will be numbered
    stringstyle=\color{mymauve},     % string literal style
    tabsize=2,	                     % sets default tabsize to 2 spaces
    title=\lstname                   % show the filename of files included with \lstinputlisting; also try caption instead of title
  }

  \usepackage{tcolorbox}
  \newtcolorbox{myquote}{colback=red!5!white, colframe=red!75!black}
  \renewenvironment{quote}{\begin{myquote}}{\end{myquote}}

---
