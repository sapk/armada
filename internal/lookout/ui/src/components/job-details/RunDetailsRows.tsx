import React, { Fragment } from "react"

import { TableCell, TableRow } from "@material-ui/core"

import { Run } from "../../services/JobService"

import "./Details.css"

interface RunDetailsRowsProps {
  run: Run
}

export function RunDetailsRows(props: RunDetailsRowsProps) {
  return (
    <Fragment>
      <TableRow className="field">
        <TableCell className="field-label">Cluster</TableCell>
        <TableCell className="field-value">{props.run.cluster}</TableCell>
      </TableRow>
      <TableRow className="field">
        <TableCell className="field-label">Pod number</TableCell>
        <TableCell className="field-value">{props.run.podNumber}</TableCell>
      </TableRow>
      <TableRow className="field">
        <TableCell className="field-label">Kubernetes Id</TableCell>
        <TableCell className="field-value">{props.run.k8sId}</TableCell>
      </TableRow>
      {props.run.node &&
      <TableRow className="field">
        <TableCell className="field-label">Cluster node</TableCell>
        <TableCell className="field-value">{props.run.node}</TableCell>
      </TableRow>}
      {props.run.podCreationTime &&
      <TableRow className="field">
        <TableCell className="field-label">Scheduled on cluster</TableCell>
        <TableCell className="field-value">{props.run.podCreationTime}</TableCell>
      </TableRow>}
      {props.run.podStartTime &&
      <TableRow className="field">
        <TableCell className="field-label">Job started</TableCell>
        <TableCell className="field-value">{props.run.podStartTime}</TableCell>
      </TableRow>}
      {props.run.finishTime &&
      <TableRow className="field">
        <TableCell className="field-label">Finished</TableCell>
        <TableCell className="field-value">{props.run.finishTime}</TableCell>
      </TableRow>}
      {props.run.error &&
      <TableRow className="field">
        <TableCell className="field-label">Error</TableCell>
        <TableCell className="field-value"><span className="error-message">{props.run.error}</span></TableCell>
      </TableRow>}
    </Fragment>
  )
}
