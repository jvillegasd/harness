package land

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// ThreadState is a review thread as read back after a write: whether it is resolved, and its latest comment.
type ThreadState struct {
	ID       string `json:"id"`
	Resolved bool   `json:"resolved"`
	Comments int    `json:"comments"`
	Last     string `json:"last"`
	URL      string `json:"url"`
}

const threadStatesQuery = `query($owner:String!,$name:String!,$number:Int!){repository(owner:$owner,name:$name){pullRequest(number:$number){reviewThreads(first:100){pageInfo{hasNextPage}nodes{id isResolved comments(last:1){totalCount nodes{body url}}}}}}}`

const replyMutation = `mutation($thread:ID!,$body:String!){addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$thread,body:$body}){comment{id}}}`

const resolveMutation = `mutation($thread:ID!){resolveReviewThread(input:{threadId:$thread}){thread{id isResolved}}}`

// threadStates reads every review thread of a pull request by id. A pull request with more than 100 threads is refused, since a thread past the first page could not be told from one that belongs elsewhere.
func (e Env) threadStates(number int) (map[string]ThreadState, error) {
	var out struct {
		Data struct {
			Repository struct {
				PullRequest struct {
					ReviewThreads struct {
						PageInfo struct{ HasNextPage bool }
						Nodes    []struct {
							ID         string
							IsResolved bool
							Comments   struct {
								TotalCount int
								Nodes      []struct{ Body, URL string }
							}
						}
					}
				}
			}
		}
	}
	if err := e.read(&out, "api", "graphql", "-f", "query="+threadStatesQuery, "-F", "owner={owner}", "-F", "name={repo}", "-F", "number="+strconv.Itoa(number)); err != nil {
		return nil, err
	}
	threads := out.Data.Repository.PullRequest.ReviewThreads
	if threads.PageInfo.HasNextPage {
		return nil, fmt.Errorf("#%d has more than 100 review threads; work them on the forge", number)
	}
	states := map[string]ThreadState{}
	for _, n := range threads.Nodes {
		s := ThreadState{ID: n.ID, Resolved: n.IsResolved, Comments: n.Comments.TotalCount}
		if last := n.Comments.Nodes; len(last) > 0 {
			s.Last, s.URL = last[0].Body, last[0].URL
		}
		states[n.ID] = s
	}
	return states, nil
}

// Reply posts the body file's exact text as a reply on one review thread of the pull request, and returns the thread as read back. The thread must belong to the pull request, and the file must hold text, before anything is written.
func Reply(e Env, number int, thread, bodyFile string) (ThreadState, error) {
	raw, err := os.ReadFile(bodyFile)
	if err != nil {
		return ThreadState{}, fmt.Errorf("cannot read the body file: %w", err)
	}
	if strings.TrimSpace(string(raw)) == "" {
		return ThreadState{}, fmt.Errorf("the body file %s is empty", bodyFile)
	}
	states, err := e.threadStates(number)
	if err != nil {
		return ThreadState{}, err
	}
	if _, ok := states[thread]; !ok {
		return ThreadState{}, fmt.Errorf("thread %s is not a review thread of #%d", thread, number)
	}
	// gh runs without a shell, so the text reaches the forge byte for byte.
	if _, err := e.Forge("api", "graphql", "-f", "query="+replyMutation, "-f", "thread="+thread, "-f", "body="+string(raw)); err != nil {
		return ThreadState{}, err
	}
	return e.readBack(number, thread)
}

// Resolve resolves the named review threads of the pull request and returns each as read back. Every id must belong to the pull request before any is resolved, and a thread already resolved is left as it is.
func Resolve(e Env, number int, threads []string) ([]ThreadState, error) {
	states, err := e.threadStates(number)
	if err != nil {
		return nil, err
	}
	var foreign []string
	for _, id := range threads {
		if _, ok := states[id]; !ok {
			foreign = append(foreign, id)
		}
	}
	if len(foreign) > 0 {
		return nil, fmt.Errorf("not review threads of #%d, so nothing was resolved: %s", number, strings.Join(foreign, ", "))
	}
	var done []string
	for _, id := range threads {
		if states[id].Resolved || slices.Contains(done, id) {
			continue
		}
		if _, err := e.Forge("api", "graphql", "-f", "query="+resolveMutation, "-f", "thread="+id); err != nil {
			return nil, err
		}
		done = append(done, id)
	}
	after, err := e.threadStates(number)
	if err != nil {
		return nil, err
	}
	out := make([]ThreadState, 0, len(threads))
	var open []string
	for _, id := range threads {
		out = append(out, after[id])
		if !after[id].Resolved {
			open = append(open, id)
		}
	}
	if len(open) > 0 {
		return out, fmt.Errorf("still unresolved after resolving: %s", strings.Join(open, ", "))
	}
	return out, nil
}

func (e Env) readBack(number int, thread string) (ThreadState, error) {
	states, err := e.threadStates(number)
	if err != nil {
		return ThreadState{}, err
	}
	s, ok := states[thread]
	if !ok {
		return ThreadState{}, errors.New("the thread is gone after the write")
	}
	return s, nil
}
