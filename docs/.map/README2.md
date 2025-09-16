# How to Publish Docs on Learn

Publishing documentation to [Learn](https://github.com/netdata/learn) involves a few key steps. Follow this guide carefully to avoid broken links or failed builds.

:warning: **Before You Begin**

- If you plan to unpublish a file, see [Unpublishing Files](#unpublishing-files) first. It requires extra steps.
- If you make large changes or move multiple docs, you must test with a local deployment of Learn.

## Steps to Publish

### Quick Checklist

| Step  | Action                                                                                  | Output                                                         |
|-------|-----------------------------------------------------------------------------------------|----------------------------------------------------------------|
| **1** | Edit `map.csv` alongside your doc changes. Fill in all 5 columns correctly.             | Docs mapped with proper sidebar labels, paths, and edit links. |
| **2** | Test locally with the `ingest.py` script. Optionally run a full local Learn deployment. | Confirms no broken links or build errors.                      |
| **3** | Merge the Docs PR (requires approval).                                                  | Docs + `map.csv` merged into the repo.                         |
| **4** | Inspect the automatic Learn ingest PR. Check files + deploy preview.                    | Verified preview of Learn with changes.                        |
| **5** | Merge the Learn ingest PR.                                                              | Docs officially live on Learn.                                 |

### 1. Edit `map.csv`

All docs must be mapped in the [map.csv](https://github.com/netdata/netdata/blob/master/docs/.map/map.csv) file. Each row has five columns:

| Column                | Purpose                                                                       | Notes                                                                                                                                                    |
|-----------------------|-------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------|
| **custom\_edit\_url** | Full GitHub **Edit** link for the file. Used for the "Edit this page" button. | Must use the full link (supports repos beyond `netdata/netdata`).                                                                                        |
| **sidebar\_label**    | The label shown in the sidebar.                                               | To make a category: the **overview** page’s `sidebar_label` **must match** `learn_rel_path` (lowercase). <br>👉 Every folder must have an overview page. |
| **learn\_status**     | `Published` or `Unpublished`.                                                 | If `Unpublished`, see [Unpublishing Files](#unpublishing-files).                                                                                         |
| **learn\_rel\_path**  | The location path on Learn.                                                   | Use **uppercase letters** and **spaces**. Example: `Netdata Agent/Installation/Linux`. <br>👉 Every level requires an overview page.                     |
| **description**       | Legacy metadata description.                                                  | Rarely used today.                                                                                                                                       |

Example row in `map.csv`:

```csv
custom_edit_url,sidebar_label,learn_status,learn_rel_path,description
https://github.com/netdata/netdata/edit/master/docs/installation/linux.md,Linux,Published,"Netdata Agent/Installation/Linux","How to install Netdata Agent on Linux"
```

### 2. Test the Changes

Before merging, **always test the map file**.

1. Clone [Learn](https://github.com/netdata/learn) locally.
2. Prepare environment and dependencies (see [ingest instructions](https://github.com/netdata/learn#ingest-and-process-documentation-files)).
3. Run the ingest command:
   ```bash
   python3 ingest/ingest.py --repos OWNEROFREPO/netdata:YOURBRANC
   ```
4. Inspect the ingested changes.
5. (Optional, advanced) [Deploy Learn](https://github.com/netdata/learn#local-deploy-of-learn) locally to confirm it builds correctly.

### 3. Merge the Docs PR

- Submit your PR with the updated docs **and** `map.csv`.
- Get at least one approval.
- **Reviewers expect you to have tested already**. Don’t rely on them to test.

### 4. Merge the Learn Ingest PR

Once your docs PR is merged:

1. The ingest action triggers in [netdata/learn](https://github.com/netdata/learn).
2. A PR is created automatically.
3. Inspect the changes.
4. Wait for the deploy preview.
5. Check the deploy preview carefully.
6. If everything looks good → merge.

🍻 Done!

## Unpublishing Files

If you **delete**, **move**, or **unpublish** a file, redirects may break.

1. Open [LegacyLearnCorrelateLinksWithGHURLs.json](https://github.com/netdata/learn/blob/master/LegacyLearnCorrelateLinksWithGHURLs.json).
2. Search (`Ctrl+F`) for the old GitHub link.
3. Update the entry to a relevant new location.
4. If no suitable replacement exists → remove the entry.
