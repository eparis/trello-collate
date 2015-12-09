package main

import (
	goflag "flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	trello "github.com/VojtechVitek/go-trello"
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

var (
	_           = fmt.Fprintf
	_           = glog.Infof
	workBuckets = strings.ToLower("Work Buckets")
	noneCard    = strings.ToLower("None")
	tagRe       = regexp.MustCompile("\\[[^\\]]+\\]") // should match [string]
)

const (
	authPathFlag   = "auth"
	configFileFlag = "config"
	periodFlag     = "period"
	onceFlag       = "once"

	checklistName      = "Open Cards"
	unknownBucketsName = "Unknown Buckets"
)

type trelloCollateBoard struct {
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	ID   string `json:"id" yaml:"id"`
}

type trelloCollateConfig struct {
	Boards  []trelloCollateBoard `json:"boards" yaml:"boards"`
	Columns []string             `json:"columns" yaml:"columns"`
}

type trelloCollateAuthConfig struct {
	AppKey string `json:"appkey" yaml:"appkey"`
	Token  string `json:"token" yaml:"token"`
}

type trelloCollate struct {
	cmd    *cobra.Command
	client *trello.Client
	auth   *trelloCollateAuthConfig
	config *trelloCollateConfig
}

func (t *trelloCollate) preCheck() error {
	configFile, err := t.cmd.Flags().GetString(configFileFlag)
	if err != nil {
		return err
	}
	if len(configFile) == 0 {
		return fmt.Errorf("--%s must be set", configFileFlag)
	}
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	t.config = &trelloCollateConfig{}
	if err = yaml.Unmarshal(data, t.config); err != nil {
		panic(err)
	}

	authPath, err := t.cmd.Flags().GetString(authPathFlag)
	if err != nil {
		return err
	}
	if len(authPath) == 0 {
		return fmt.Errorf("Must set --%s", authPathFlag)
	}
	data, err = ioutil.ReadFile(authPath)
	if err != nil {
		return fmt.Errorf("error reading auth file %q: %v", authPath, err)
	}

	t.auth = &trelloCollateAuthConfig{}
	if err = yaml.Unmarshal(data, t.auth); err != nil {
		return nil
	}

	transport := trello.NewBearerTokenTransport(t.auth.AppKey, t.auth.Token)
	client, err := trello.NewClient(transport)
	if err != nil {
		return err
	}
	t.client = client
	return nil
}

func (t *trelloCollate) getBucketsCards(lists map[string]trello.List) (map[string]trello.Card, error) {
	out := map[string]trello.Card{}
	list, ok := lists[workBuckets]
	// If there isn't a list called 'workBuckets' we have no buckets, so return
	if !ok {
		return out, nil
	}
	cards, err := list.Cards()
	if err != nil {
		return nil, err
	}
	for _, card := range cards {
		name := strings.ToLower(card.Name)
		out[name] = card
	}
	return out, nil
}

func addCardToBuckets(card trello.Card, bucketCards map[string]trello.Card, mapping map[string][]trello.Card) map[string][]trello.Card {
	title := strings.ToLower(card.Name)
	found := false
	tags := tagRe.FindAllString(title, -1)
	for _, wholeTag := range tags {
		tag := wholeTag[1 : len(wholeTag)-1] // transform '[tag]' to just 'tag'
		tag = strings.ToLower(tag)
		mapping[tag] = append(mapping[tag], card)
		if _, ok := bucketCards[tag]; ok {
			found = true
		}
	}
	if !found {
		mapping[noneCard] = append(mapping[noneCard], card)
	}
	return mapping
}

func (t *trelloCollate) getCardsForBuckets(lists map[string]trello.List, bucketCards map[string]trello.Card) (map[string][]trello.Card, error) {
	out := map[string][]trello.Card{}
	for _, listName := range t.config.Columns {
		list, ok := lists[listName]
		if !ok {
			continue
		}
		cards, err := list.Cards()
		if err != nil {
			return nil, err
		}
		for _, card := range cards {
			out = addCardToBuckets(card, bucketCards, out)
		}
	}
	return out, nil
}

func clearChecklist(list trello.Checklist) error {
	for _, item := range list.CheckItems {
		if err := item.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func updateBcard(bcard trello.Card, cards []trello.Card) error {
	items := []string{}
	for _, card := range cards {
		items = append(items, card.Url)
	}

	return setChecklist(bcard, checklistName, items)
}

func setChecklist(card trello.Card, listName string, items []string) error {
	lowerListName := strings.ToLower(listName)
	var checklist *trello.Checklist

	checklists, err := card.Checklists()
	if err != nil {
		return err
	}
	for i := range checklists {
		l := &checklists[i]
		name := strings.ToLower(l.Name)
		if name == lowerListName {
			checklist = l
			break
		}
	}

	if checklist == nil {
		checklist, err = card.AddChecklist(listName)
		if err != nil {
			return err
		}
	}

	current := map[string]trello.ChecklistItem{}
	for _, item := range checklist.CheckItems {
		current[item.Name] = item
	}

	needed := map[string]struct{}{}
	for _, item := range items {
		needed[item] = struct{}{}
	}

	// remove cards that don't belong there
	for item, checklistItem := range current {
		if _, exists := needed[item]; exists {
			continue
		}
		if err := checklistItem.Delete(); err != nil {
			return err
		}
	}

	// add cards that are missing
	for item := range needed {
		if _, exists := current[item]; exists {
			continue
		}
		if _, err := checklist.AddItem(item, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func updateUnknownBuckets(bucketCards map[string]trello.Card, cardsForBucket map[string][]trello.Card) error {
	none := bucketCards[noneCard]

	items := []string{}

	for bucket := range cardsForBucket {
		// if we already have the given bucket we don't need to record
		if _, ok := bucketCards[bucket]; ok {
			continue
		}
		items = append(items, bucket)
	}
	if err := setChecklist(none, unknownBucketsName, items); err != nil {
		return err
	}
	return nil
}

func (t *trelloCollate) processBoard(board *trello.Board) error {
	lists := map[string]trello.List{}

	ls, err := board.Lists()
	if err != nil {
		return err
	}
	for _, list := range ls {
		name := strings.ToLower(list.Name)
		lists[name] = list
	}

	bucketCards, err := t.getBucketsCards(lists)
	if err != nil {
		return err
	}

	cardsForBucket, err := t.getCardsForBuckets(lists, bucketCards)
	if err != nil {
		return err
	}

	for bucket, bcard := range bucketCards {
		if err := updateBcard(bcard, cardsForBucket[bucket]); err != nil {
			return err
		}
	}

	// if a none card, we can record unknown buckets
	if _, ok := bucketCards[noneCard]; ok {
		if err := updateUnknownBuckets(bucketCards, cardsForBucket); err != nil {
			return err
		}
	}
	return nil
}

func (t *trelloCollate) mainLoop() error {
	for {
		period, _ := t.cmd.Flags().GetDuration(periodFlag)
		nextRunStartTime := time.Now().Add(period)

		for _, b := range t.config.Boards {
			board, err := t.client.Board(b.ID)
			if err != nil {
				return err
			}
			if err = t.processBoard(board); err != nil {
				return err
			}
		}
		once, _ := t.cmd.Flags().GetBool(onceFlag)
		if once {
			break
		}
		sleepDuration := nextRunStartTime.Sub(time.Now())
		glog.Infof("Sleeping for %v\n", sleepDuration)
		time.Sleep(sleepDuration)
	}
	return nil
}

func main() {
	t := &trelloCollate{}
	cmd := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "A program to add cards to lists on other cards",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			t.cmd = cmd
			return t.preCheck()
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return t.mainLoop()
		},
	}
	cmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)
	cmd.PersistentFlags().String(authPathFlag, "auth.yaml", "Location of auth token and app key.")
	cmd.Flags().String(configFileFlag, "config.yaml", "Config file telling what boards and columns to update.")
	cmd.Flags().Bool(onceFlag, false, "Should this command run one time or every period?")
	cmd.Flags().Duration(periodFlag, 30*time.Minute, "How often we should update cards")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
