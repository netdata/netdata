# How to Publish Docs on Learn

Here is the forbidden knowledge of how one summons docs from this repo to the production Documentation site, Learn. This process involves code magic, you have been warned 😄

## Prerequisites

> [!CAUTION]
> If you will **unpublish** a file that was on Learn, refer to [this section](#unpublishing-files) as it is a necessary step

1. If the change is not obvious, and a lot of stuff is moved around, a local deployment of <https://github.com/netdata/learn> is needed. This is a must and a PR submitted for review should first be tested

## Steps

### 1. Edit the map.csv in the same PR as the docs you are editing

The map file has five columns.

#### custom_edit_url

This column has the link for the github file you want to use, due to having files from other repos apart from netdata/netdata, use full **EDIT** link here. This link will be used when someone presses the edit button on Learn.

#### sidebar_Label

The label that the doc will have on the sidebar.

> [!Important]
>
> If you want to make a category, then the **overview** page has to have `sidebar_label` == `learn_rel_path`, otherwise it will not work and be lowercase. This is by design, as we autogenerate the sidebar and keep it dynamic.
>
> **Folders must have an overview page**

#### learn_status

`Published` or `Unpublished`.

> [!Caution]
>
> If you flip something to unpublished please refer to [this section](#unpublishing-files).

#### learn_rel_path

This is the path that the file must go to, use uppercase letters, spaces and separate with slashes.

Example: `Netdata Agent/Installation/Linux`

> [!Note]
>
> For every level you go, you need an overview page as well

#### description

This column is sort of legacy, it is meant to populate a `description` metadata field.

### 2. Test the changes

> [!Important]
>
> If you don't want to do this process all over again because you made a wrong edit and somehow you merged it, you should **test** the map file.

This is heavily documented in <https://github.com/netdata/learn#ingest-and-process-documentation-files>, but the gist is:

On a local clone of Learn, **after following the ingest instructions detailed in the above link** (prep environment, pip dependencies) you run:

```bash
python3 ingest/ingest.py --repos OWNEROFREPO/netdata:YOURBRANCH
```

> you can override all the ingested repos, but most of the time you would be concerned about netdata/netdata.

**You then inspect the changes.**

>[!Important]
>
>If you are a real legend you can also do a [local deploy of Learn](https://github.com/netdata/learn#local-deploy-of-learn) to make sure the thing builds *(looking at you, that you did these HTML changes and have syntax errors)*.

### 3. Merge the PR

After you make the edits in the CSV, you merge the Docs PR. You need an approval for this, so the map will be reviewed there.

>[!Note]
>
> The review is done after you have tested your changes, it is weird to commit something untested, and don't expect the reviewer to test it for you

### 4. Merge the Learn ingest PR

When edits are made in `netdata/netdata` the ingest action is triggered at `netdata/learn`. Upon success it will make a PR like [this](https://github.com/netdata/learn/pull/2551).

1. Inspect changes in files
2. Wait for deploy preview
3. Check the deploy preview

If everything looks good, you merge.

End of story, :beers:

## Unpublishing Files

If you:

1. **delete** a file from a repo
2. move it (which 404s the old link)  
3. unpublish it

**you will break redirects to it.**

The entries for it in the [redirects JSON](https://github.com/netdata/learn/blob/master/LegacyLearnCorrelateLinksWithGHURLs.json) will point to the 404.

To fix, you just ctrl+f in that file for the old GitHub link, and either point it somewhere related. If no relevant spot can be found, then just remove the entries.
