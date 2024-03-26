all
rule 'MD003', :style => :atx      # Header style. 'atx' is the classic hash style headers
rule 'MD004', :style => :dash     # Unordered list character style, we always use `-`.
rule 'MD013', :line_length => 150 # Max line length
rule 'MD046', :style => :fenced   # Code block style, we always use fenced (``` at top and bottom).

exclude_rule 'MD033'              # Allow inline HTML
