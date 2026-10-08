# Triage PR Duplication Gate Failures

Use this when `SonarCloud Code Analysis` fails only because
`new_duplicated_lines_density` exceeds the quality gate.

## Steps

1. Check the quality gate:

   ```sh
   curl -fsS \
     "https://sonarcloud.io/api/qualitygates/project_status?projectKey=${SONAR_PROJECT}&pullRequest=${PR}" |
     jq -r '.projectStatus.status, (.projectStatus.conditions[]? | [.metricKey, .status, (.actualValue // ""), (.errorThreshold // "")] | @tsv)'
   ```

2. Fetch file-level new-code duplication measures:

   ```sh
   curl -fsS \
     "https://sonarcloud.io/api/measures/component_tree?component=${SONAR_PROJECT}&pullRequest=${PR}&metricKeys=new_duplicated_lines_density,new_duplicated_lines,new_lines&qualifiers=FIL&ps=500" \
     > /tmp/sonar-pr-files.json
   ```

3. Rank duplicated files. PR measures are in `periods[0].value`:

   ```sh
   jq -r '
     def pval($m): ((.measures[]? | select(.metric==$m) | .periods[0].value) // "0");
     .components[]
     | {
         path,
         dup:(pval("new_duplicated_lines")|tonumber),
         density:(pval("new_duplicated_lines_density")|tonumber),
         lines:(pval("new_lines")|tonumber)
       }
     | select(.dup > 0)
     | [.dup, .density, .lines, .path]
     | @tsv
   ' /tmp/sonar-pr-files.json | sort -nr
   ```

4. Fetch duplication blocks for the largest contributors:

   ```sh
   curl -fsS --get "https://sonarcloud.io/api/duplications/show" \
     --data-urlencode "key=${SONAR_PROJECT}:path/to/file.go" \
     --data-urlencode "pullRequest=${PR}" |
     jq .
   ```

5. Prefer small test-helper refactors or generated-code fixes when the blocks
   are real duplication. Do not add filler lines just to move the percentage.
