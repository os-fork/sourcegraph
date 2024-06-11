import { dirname } from 'path'

import { BehaviorSubject, concatMap, from, map } from 'rxjs'

import { type BlameHunkData, fetchBlameHunksMemoized } from '@sourcegraph/web/src/repo/blame/shared'

import { SourcegraphURL } from '$lib/common'
import { getGraphQLClient, mapOrThrow } from '$lib/graphql'
import { resolveRevision } from '$lib/repo/utils'
import { parseRepoRevision } from '$lib/shared'
import { assertNonNullable } from '$lib/utils'

import type { PageLoad, PageLoadEvent } from './$types'
import {
    BlobDiffViewCommitQuery,
    BlobFileViewBlobQuery,
    BlobFileViewCommitQuery_revisionOverride,
    BlobFileViewHighlightedFileQuery,
} from './page.gql'

function loadDiffView({ params, url }: PageLoadEvent) {
    const client = getGraphQLClient()
    const revisionOverride = url.searchParams.get('rev')
    const { repoName } = parseRepoRevision(params.repo)
    const filePath = decodeURIComponent(params.path)

    assertNonNullable(revisionOverride, 'revisionOverride is set')

    return {
        type: 'DiffView' as const,
        enableInlineDiff: true,
        enableViewAtCommit: true,
        filePath,
        commit: client
            .query(BlobDiffViewCommitQuery, {
                repoName,
                revspec: revisionOverride,
                path: filePath,
            })
            .then(mapOrThrow(result => result.data?.repository?.commit ?? null)),
    }
}

async function loadFileView({ parent, params, url }: PageLoadEvent) {
    const client = getGraphQLClient()
    const revisionOverride = url.searchParams.get('rev')
    const isBlame = url.searchParams.get('view') === 'blame'
    const lineOrPosition = SourcegraphURL.from(url).lineRange
    const { repoName, revision = '' } = parseRepoRevision(params.repo)
    const resolvedRevision = revisionOverride ? Promise.resolve(revisionOverride) : resolveRevision(parent, revision)
    const filePath = decodeURIComponent(params.path)

    // Create a BehaviorSubject so preloading does not create a subscriberless observable
    const blameData = new BehaviorSubject<BlameHunkData>({ current: undefined, externalURLs: undefined })
    if (isBlame) {
        const blameHunks = from(resolvedRevision).pipe(
            concatMap(resolvedRevision => fetchBlameHunksMemoized({ repoName, revision: resolvedRevision, filePath }))
        )

        from(parent())
            .pipe(
                concatMap(({ resolvedRevision }) =>
                    blameHunks.pipe(
                        map(blameHunks => ({
                            externalURLs: resolvedRevision.repo.externalURLs,
                            current: blameHunks,
                        }))
                    )
                )
            )
            .subscribe(v => blameData.next(v))
    }

    const blob = resolvedRevision
        .then(resolvedRevision =>
            client.query(BlobFileViewBlobQuery, {
                repoName,
                revspec: resolvedRevision,
                path: filePath,
            })
        )
        .then(mapOrThrow(result => result.data?.repository?.commit?.blob ?? null))

    parent().then(parent =>
        parent.repositoryContext.set({
            directoryPath: dirname(filePath),
            filePath,
        })
    )
    blob.then(fileData => {
        const fileLanguage = fileData?.languages?.[0]
        if (fileLanguage) {
            parent().then(parent => parent.repositoryContext.update(context => ({ ...context, fileLanguage })))
        }
    })

    return {
        type: 'FileView' as const,
        enableInlineDiff: true,
        enableViewAtCommit: true,
        graphQLClient: client,
        lineOrPosition,
        filePath,
        blob,
        highlights: resolvedRevision
            .then(resolvedRevision =>
                client.query(BlobFileViewHighlightedFileQuery, {
                    repoName,
                    revspec: resolvedRevision,
                    path: filePath,
                    disableTimeout: false,
                })
            )
            .then(mapOrThrow(result => result.data?.repository?.commit?.blob?.highlight ?? null)),
        // We can ignore the error because if the revision doesn't exist, other queries will fail as well
        revisionOverride: revisionOverride
            ? await client
                  .query(BlobFileViewCommitQuery_revisionOverride, {
                      repoName,
                      revspec: revisionOverride,
                  })
                  .then(result => result.data?.repository?.commit)
            : null,
        externalServiceType: parent()
            .then(({ resolvedRevision }) => resolvedRevision.repo?.externalRepository?.serviceType)
            .catch(error => {
                console.error('Failed to fetch repository data:', error)
                return null
            }),
        blameData,
    }
}

export const load: PageLoad = event => {
    const showDiff = event.url.searchParams.has('diff')
    const revisionOverride = event.url.searchParams.get('rev')

    if (showDiff && revisionOverride) {
        return loadDiffView(event)
    }
    return loadFileView(event)
}
