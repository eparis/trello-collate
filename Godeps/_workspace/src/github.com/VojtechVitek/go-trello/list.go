/*
Copyright 2014 go-trello authors. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package trello

import (
	"encoding/json"
)

type List struct {
	client  *Client
	Id      string  `json:"id"`
	Name    string  `json:"name"`
	Closed  bool    `json:"closed"`
	IdBoard string  `json:"idBoard"`
	Pos     float32 `json:"pos"`
}

func (l *List) Cards() (cards []Card, err error) {
	body, err := l.client.Get("/lists/" + l.Id + "/cards")
	if err != nil {
		return
	}

	err = json.Unmarshal(body, &cards)
	for i := range cards {
		cards[i].client = l.client
	}
	return
}
