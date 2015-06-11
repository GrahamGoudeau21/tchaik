"use strict";

import React from "react";

import SearchStore from "../stores/SearchStore.js";

import {GroupList as GroupList} from "./Collection.js";
import CollectionStore from "../stores/CollectionStore.js";
import CollectionActions from "../actions/CollectionActions.js";

import Icon from "./Icon.js";

export class Search extends React.Component {
  render() {
    return (
      <div className="collection">
        <Results />
      </div>
    );
  }
}


function getResultsState() {
  return {results: SearchStore.getResults()};
}

class Results extends React.Component {
  constructor(props) {
    super(props);

    this.state = getResultsState();
    this._onChange = this._onChange.bind(this);
  }

  componentDidMount() {
    SearchStore.addChangeListener(this._onChange);
  }

  componentWillUnmount() {
    SearchStore.removeChangeListener(this._onChange);
  }

  render() {
    var list = this.state.results;
    if (list.length === 0) {
      return (
        <div className="collection">
          <div className="no-results"><Icon icon="headphones" />No results found</div>
        </div>
      );
    }
    return <GroupList path={["Root"]} list={list} depth={0} />;
  }

  _onChange() {
    this.setState(getResultsState());
  }
}


function getItem(path) {
  var c = CollectionStore.getCollection(path);
  if (c === undefined) {
    return null;
  }
  return c;
}

function getRootGroupState(props) {
  return {item: getItem(props.path)};
}

export class RootGroup extends React.Component {
  constructor(props) {
    super(props);

    this.state = getRootGroupState(this.props);
    this._onChange = this._onChange.bind(this);
  }

  componentDidMount() {
    CollectionStore.addChangeListener(this._onChange);
    CollectionActions.fetch(this.props.path);
  }

  componentWillUnmount() {
    CollectionStore.removeChangeListener(this._onChange);
  }

  render() {
    if (this.state.item === null) {
      return null;
    }

    return (
      <Group item={this.state.item} path={this.props.path} depth={1} />
    );
  }

  _onChange(keyPath) {
    if (CollectionStore.pathToKey(this.props.path) === keyPath) {
      this.setState(getRootGroupState(this.props));
    }
  }
}

RootGroup.propTypes = {
  path: React.PropTypes.array.isRequired,
};
